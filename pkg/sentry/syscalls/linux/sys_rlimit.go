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

package linux

import (
	"github.com/talismancer/gvisor-ligolo/pkg/abi/linux"
	"github.com/talismancer/gvisor-ligolo/pkg/errors/linuxerr"
	"github.com/talismancer/gvisor-ligolo/pkg/hostarch"
	"github.com/talismancer/gvisor-ligolo/pkg/marshal"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/arch"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/kernel"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/limits"
)

// rlimit describes an implementation of 'struct rlimit', which may vary from
// system-to-system.
type rlimit interface {
	marshal.Marshallable

	// toLimit converts an rlimit to a limits.Limit.
	toLimit() *limits.Limit

	// fromLimit converts a limits.Limit to an rlimit.
	fromLimit(lim limits.Limit)
}

// newRlimit returns the appropriate rlimit type for 'struct rlimit' on this system.
func newRlimit(t *kernel.Task) (rlimit, error) {
	switch t.Arch().Width() {
	case 8:
		// On 64-bit system, struct rlimit and struct rlimit64 are identical.
		return &rlimit64{}, nil
	default:
		return nil, linuxerr.ENOSYS
	}
}

// +marshal
type rlimit64 struct {
	Cur uint64
	Max uint64
}

func (r *rlimit64) toLimit() *limits.Limit {
	return &limits.Limit{
		Cur: limits.FromLinux(r.Cur),
		Max: limits.FromLinux(r.Max),
	}
}

func (r *rlimit64) fromLimit(lim limits.Limit) {
	*r = rlimit64{
		Cur: limits.ToLinux(lim.Cur),
		Max: limits.ToLinux(lim.Max),
	}
}

func (r *rlimit64) copyIn(t *kernel.Task, addr hostarch.Addr) error {
	_, err := r.CopyIn(t, addr)
	return err
}

func (r *rlimit64) copyOut(t *kernel.Task, addr hostarch.Addr) error {
	_, err := r.CopyOut(t, addr)
	return err
}

func makeRlimit64(lim limits.Limit) *rlimit64 {
	return &rlimit64{Cur: lim.Cur, Max: lim.Max}
}

// setableLimits is the set of supported setable limits.
var setableLimits = map[limits.LimitType]struct{}{
	limits.NumberOfFiles: {},
	limits.AS:            {},
	limits.CPU:           {},
	limits.Data:          {},
	limits.FileSize:      {},
	limits.MemoryLocked:  {},
	limits.Stack:         {},
	// RSS can be set, but it's not enforced because Linux doesn't enforce it
	// either: "This limit has effect only in Linux 2.4.x, x < 30"
	limits.Rss: {},
	// These are not enforced, but we include them here to avoid returning
	// EPERM, since some apps expect them to succeed.
	limits.Core:         {},
	limits.ProcessCount: {},
}

func prlimit64(t *kernel.Task, resource limits.LimitType, newLim *limits.Limit) (limits.Limit, error) {
	if newLim == nil {
		return t.ThreadGroup().Limits().Get(resource), nil
	}

	if _, ok := setableLimits[resource]; !ok {
		return limits.Limit{}, linuxerr.EPERM
	}

	// "A privileged process (under Linux: one with the CAP_SYS_RESOURCE
	// capability in the initial user namespace) may make arbitrary changes
	// to either limit value."
	privileged := t.HasCapabilityIn(linux.CAP_SYS_RESOURCE, t.Kernel().RootUserNamespace())

	oldLim, err := t.ThreadGroup().Limits().Set(resource, *newLim, privileged)
	if err != nil {
		return limits.Limit{}, err
	}

	if resource == limits.CPU {
		t.NotifyRlimitCPUUpdated()
	}
	return oldLim, nil
}

// Getrlimit implements linux syscall getrlimit(2).
func Getrlimit(t *kernel.Task, sysno uintptr, args arch.SyscallArguments) (uintptr, *kernel.SyscallControl, error) {
	resource, ok := limits.FromLinuxResource[int(args[0].Int())]
	if !ok {
		// Return err; unknown limit.
		return 0, nil, linuxerr.EINVAL
	}
	addr := args[1].Pointer()
	rlim, err := newRlimit(t)
	if err != nil {
		return 0, nil, err
	}
	lim, err := prlimit64(t, resource, nil)
	if err != nil {
		return 0, nil, err
	}
	rlim.fromLimit(lim)
	_, err = rlim.CopyOut(t, addr)
	return 0, nil, err
}

// Setrlimit implements linux syscall setrlimit(2).
func Setrlimit(t *kernel.Task, sysno uintptr, args arch.SyscallArguments) (uintptr, *kernel.SyscallControl, error) {
	resource, ok := limits.FromLinuxResource[int(args[0].Int())]
	if !ok {
		// Return err; unknown limit.
		return 0, nil, linuxerr.EINVAL
	}
	addr := args[1].Pointer()
	rlim, err := newRlimit(t)
	if err != nil {
		return 0, nil, err
	}
	if _, err := rlim.CopyIn(t, addr); err != nil {
		return 0, nil, linuxerr.EFAULT
	}
	_, err = prlimit64(t, resource, rlim.toLimit())
	return 0, nil, err
}

// Prlimit64 implements linux syscall prlimit64(2).
func Prlimit64(t *kernel.Task, sysno uintptr, args arch.SyscallArguments) (uintptr, *kernel.SyscallControl, error) {
	tid := kernel.ThreadID(args[0].Int())
	resource, ok := limits.FromLinuxResource[int(args[1].Int())]
	if !ok {
		// Return err; unknown limit.
		return 0, nil, linuxerr.EINVAL
	}
	newRlimAddr := args[2].Pointer()
	oldRlimAddr := args[3].Pointer()

	var newLim *limits.Limit
	if newRlimAddr != 0 {
		var nrl rlimit64
		if err := nrl.copyIn(t, newRlimAddr); err != nil {
			return 0, nil, linuxerr.EFAULT
		}
		newLim = nrl.toLimit()
	}

	if tid < 0 {
		return 0, nil, linuxerr.EINVAL
	}
	ot := t
	if tid > 0 {
		if ot = t.PIDNamespace().TaskWithID(tid); ot == nil {
			return 0, nil, linuxerr.ESRCH
		}
	}

	// "To set or get the resources of a process other than itself, the caller
	// must have the CAP_SYS_RESOURCE capability, or the real, effective, and
	// saved set user IDs of the target process must match the real user ID of
	// the caller and the real, effective, and saved set group IDs of the
	// target process must match the real group ID of the caller."
	if ot != t && !t.HasCapabilityIn(linux.CAP_SYS_RESOURCE, t.PIDNamespace().UserNamespace()) {
		cred, tcred := t.Credentials(), ot.Credentials()
		if cred.RealKUID != tcred.RealKUID ||
			cred.RealKUID != tcred.EffectiveKUID ||
			cred.RealKUID != tcred.SavedKUID ||
			cred.RealKGID != tcred.RealKGID ||
			cred.RealKGID != tcred.EffectiveKGID ||
			cred.RealKGID != tcred.SavedKGID {
			return 0, nil, linuxerr.EPERM
		}
	}

	oldLim, err := prlimit64(ot, resource, newLim)
	if err != nil {
		return 0, nil, err
	}

	if oldRlimAddr != 0 {
		if err := makeRlimit64(oldLim).copyOut(t, oldRlimAddr); err != nil {
			return 0, nil, linuxerr.EFAULT
		}
	}

	return 0, nil, nil
}
