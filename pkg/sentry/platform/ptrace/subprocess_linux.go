// Copyright 2018 The gVisor Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build linux
// +build linux

package ptrace

import (
	"fmt"

	"github.com/talismancer/gvisor-ligolo/pkg/abi/linux"
	"github.com/talismancer/gvisor-ligolo/pkg/hosttid"
	"github.com/talismancer/gvisor-ligolo/pkg/log"
	"github.com/talismancer/gvisor-ligolo/pkg/seccomp"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/arch"
	"golang.org/x/sys/unix"
)

const syscallEvent unix.Signal = 0x80

// createStub creates a fresh stub processes.
//
// Precondition: the runtime OS thread must be locked.
func createStub() (*thread, error) {
	// The exact interactions of ptrace and seccomp are complex, and
	// changed in recent kernel versions. Before commit 93e35efb8de45, the
	// seccomp check is done before the ptrace emulation check. This means
	// that any calls not matching this list will trigger the seccomp
	// default action instead of notifying ptrace.
	//
	// After commit 93e35efb8de45, the seccomp check is done after the
	// ptrace emulation check. This simplifies using SYSEMU, since seccomp
	// will never run for emulation. Seccomp will only run for injected
	// system calls, and thus we can use RET_KILL as our violation action.
	var defaultAction linux.BPFAction
	if probeSeccomp() {
		log.Infof("Latest seccomp behavior found (kernel >= 4.8 likely)")
		defaultAction = linux.SECCOMP_RET_KILL_THREAD
	} else {
		// We must rely on SYSEMU behavior; tracing with SYSEMU is broken.
		log.Infof("Legacy seccomp behavior found (kernel < 4.8 likely)")
		defaultAction = linux.SECCOMP_RET_ALLOW
	}

	// When creating the new child process, we specify SIGKILL as the
	// signal to deliver when the child exits. We never expect a subprocess
	// to exit; they are pooled and reused. This is done to ensure that if
	// a subprocess is OOM-killed, this process (and all other stubs,
	// transitively) will be killed as well. It's simply not possible to
	// safely handle a single stub getting killed: the exact state of
	// execution is unknown and not recoverable.
	//
	// In addition, we set the PTRACE_O_TRACEEXIT option to log more
	// information about a stub process when it receives a fatal signal.
	return attachedThread(uintptr(unix.SIGKILL)|unix.CLONE_FILES, defaultAction)
}

// attachedThread returns a new attached thread.
//
// Precondition: the runtime OS thread must be locked.
func attachedThread(flags uintptr, defaultAction linux.BPFAction) (*thread, error) {
	// Create a BPF program that allows only the system calls needed by the
	// stub and all its children. This is used to create child stubs
	// (below), so we must include the ability to fork, but otherwise lock
	// down available calls only to what is needed.
	rules := []seccomp.RuleSet{}
	if defaultAction != linux.SECCOMP_RET_ALLOW {
		rules = append(rules, seccomp.RuleSet{
			Rules: seccomp.SyscallRules{
				unix.SYS_CLONE: []seccomp.Rule{
					// Allow creation of new subprocesses (used by the master).
					{seccomp.EqualTo(unix.CLONE_FILES | unix.SIGKILL)},
					// Allow creation of new threads within a single address space (used by addresss spaces).
					{seccomp.EqualTo(
						unix.CLONE_FILES |
							unix.CLONE_FS |
							unix.CLONE_SIGHAND |
							unix.CLONE_THREAD |
							unix.CLONE_PTRACE |
							unix.CLONE_VM)},
				},

				// For the initial process creation.
				unix.SYS_WAIT4: {},
				unix.SYS_EXIT:  {},

				// For the stub prctl dance (all).
				unix.SYS_PRCTL: []seccomp.Rule{
					{seccomp.EqualTo(unix.PR_SET_PDEATHSIG), seccomp.EqualTo(unix.SIGKILL)},
				},
				unix.SYS_GETPPID: {},

				// For the stub to stop itself (all).
				unix.SYS_GETPID: {},
				unix.SYS_KILL: []seccomp.Rule{
					{seccomp.MatchAny{}, seccomp.EqualTo(unix.SIGSTOP)},
				},

				// Injected to support the address space operations.
				unix.SYS_MMAP:   {},
				unix.SYS_MUNMAP: {},
			},
			Action: linux.SECCOMP_RET_ALLOW,
		})
	}
	rules = appendArchSeccompRules(rules, defaultAction)
	instrs, err := seccomp.BuildProgram(rules, defaultAction, defaultAction)
	if err != nil {
		return nil, err
	}

	return forkStub(flags, instrs)
}

// In the child, this function must not acquire any locks, because they might
// have been locked at the time of the fork. This means no rescheduling, no
// malloc calls, and no new stack segments.  For the same reason compiler does
// not race instrument it.
//
//go:norace
func forkStub(flags uintptr, instrs []linux.BPFInstruction) (*thread, error) {
	// Declare all variables up front in order to ensure that there's no
	// need for allocations between beforeFork & afterFork.
	var (
		pid   uintptr
		ppid  uintptr
		errno unix.Errno
	)

	// Remember the current ppid for the pdeathsig race.
	ppid, _, _ = unix.RawSyscall(unix.SYS_GETPID, 0, 0, 0)

	// Among other things, beforeFork masks all signals.
	beforeFork()

	// Do the clone.
	pid, _, errno = unix.RawSyscall6(unix.SYS_CLONE, flags, 0, 0, 0, 0, 0)
	if errno != 0 {
		afterFork()
		return nil, errno
	}

	// Is this the parent?
	if pid != 0 {
		// Among other things, restore signal mask.
		afterFork()

		// Initialize the first thread.
		t := &thread{
			tgid: int32(pid),
			tid:  int32(pid),
			cpu:  ^uint32(0),
		}
		if sig := t.wait(stopped); sig != unix.SIGSTOP {
			return nil, fmt.Errorf("wait failed: expected SIGSTOP, got %v", sig)
		}
		t.attach()
		t.grabInitRegs()

		return t, nil
	}

	// Move the stub to a new session (and thus a new process group). This
	// prevents the stub from getting PTY job control signals intended only
	// for the sentry process. We must call this before restoring signal
	// mask.
	if _, _, errno := unix.RawSyscall(unix.SYS_SETSID, 0, 0, 0); errno != 0 {
		unix.RawSyscall(unix.SYS_EXIT, uintptr(errno), 0, 0)
	}

	// afterForkInChild resets all signals to their default dispositions
	// and restores the signal mask to its pre-fork state.
	afterForkInChild()

	// Explicitly unmask all signals to ensure that the tracer can see
	// them.
	if errno := unmaskAllSignals(); errno != 0 {
		unix.RawSyscall(unix.SYS_EXIT, uintptr(errno), 0, 0)
	}

	// Set an aggressive BPF filter for the stub and all it's children. See
	// the description of the BPF program built above.
	if errno := seccomp.SetFilterInChild(instrs); errno != 0 {
		unix.RawSyscall(unix.SYS_EXIT, uintptr(errno), 0, 0)
	}

	// Enable cpuid-faulting.
	enableCpuidFault()

	// Call the stub; should not return.
	stubCall(stubStart, ppid)
	panic("unreachable")
}

// createStub creates a stub processes as a child of an existing subprocesses.
//
// Precondition: the runtime OS thread must be locked.
func (s *subprocess) createStub() (*thread, error) {
	// There's no need to lock the runtime thread here, as this can only be
	// called from a context that is already locked.
	currentTID := int32(hosttid.Current())
	t := s.syscallThreads.lookupOrCreate(currentTID, s.newThread)

	// Pass the expected PPID to the child via R15.
	regs := t.initRegs
	initChildProcessPPID(&regs, t.tgid)

	// Call fork in a subprocess.
	//
	// The new child must set up PDEATHSIG to ensure it dies if this
	// process dies. Since this process could die at any time, this cannot
	// be done via instrumentation from here.
	//
	// Instead, we create the child untraced, which will do the PDEATHSIG
	// setup and then SIGSTOP itself for our attach below.
	//
	// See above re: SIGKILL.
	pid, err := t.syscallIgnoreInterrupt(
		&regs,
		unix.SYS_CLONE,
		arch.SyscallArgument{Value: uintptr(unix.SIGKILL | unix.CLONE_FILES)},
		arch.SyscallArgument{Value: 0},
		arch.SyscallArgument{Value: 0},
		arch.SyscallArgument{Value: 0},
		arch.SyscallArgument{Value: 0},
		arch.SyscallArgument{Value: 0})
	if err != nil {
		return nil, fmt.Errorf("creating stub process: %v", err)
	}

	// Wait for child to enter group-stop, so we don't stop its
	// bootstrapping work with t.attach below.
	//
	// We unfortunately don't have a handy part of memory to write the wait
	// status. If the wait succeeds, we'll assume that it was the SIGSTOP.
	// If the child actually exited, the attach below will fail.
	_, err = t.syscallIgnoreInterrupt(
		&t.initRegs,
		unix.SYS_WAIT4,
		arch.SyscallArgument{Value: uintptr(pid)},
		arch.SyscallArgument{Value: 0},
		arch.SyscallArgument{Value: unix.WALL | unix.WUNTRACED},
		arch.SyscallArgument{Value: 0},
		arch.SyscallArgument{Value: 0},
		arch.SyscallArgument{Value: 0})
	if err != nil {
		return nil, fmt.Errorf("waiting on stub process: %v", err)
	}

	childT := &thread{
		tgid: int32(pid),
		tid:  int32(pid),
		cpu:  ^uint32(0),
	}
	childT.attach()

	return childT, nil
}
