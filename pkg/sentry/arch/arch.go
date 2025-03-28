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

// Package arch provides abstractions around architecture-dependent details,
// such as syscall calling conventions, native types, etc.
package arch

import (
	"fmt"
	"io"

	"github.com/talismancer/gvisor-ligolo/pkg/abi/linux"
	"github.com/talismancer/gvisor-ligolo/pkg/cpuid"
	"github.com/talismancer/gvisor-ligolo/pkg/hostarch"
	"github.com/talismancer/gvisor-ligolo/pkg/marshal"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/arch/fpu"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/limits"
)

// Arch describes an architecture.
type Arch int

const (
	// AMD64 is the x86-64 architecture.
	AMD64 Arch = iota
	// ARM64 is the aarch64 architecture.
	ARM64
)

// String implements fmt.Stringer.
func (a Arch) String() string {
	switch a {
	case AMD64:
		return "amd64"
	case ARM64:
		return "arm64"
	default:
		return fmt.Sprintf("Arch(%d)", a)
	}
}

// contextInterface provides architecture-dependent information for a thread.
// This is currently not referenced, because there exists only one concrete
// implementation of this interface (*Context64), which we reference directly
// wherever this interface could otherwise be used in order to avoid the
// overhead involved in calling functions on interfaces in Go.
// This interface is still useful in order to see the entire
// architecture-dependent call surface it must support, as this is difficult
// to follow across the rest of this module due to the conditional compilation
// of the files that make it up.
//
// NOTE(b/34169503): Currently we use uintptr here to refer to a generic native
// register value. While this will work for the foreseeable future, it isn't
// strictly correct. We may want to create some abstraction that makes this
// more clear or enables us to store values of arbitrary widths. This is
// particularly true for RegisterMap().
type contextInterface interface {
	// Arch returns the architecture for this Context.
	Arch() Arch

	// Native converts a generic type to a native value.
	//
	// Because the architecture is not specified here, we may be dealing
	// with return values of varying sizes (for example ARCH_GETFS). This
	// is a simple utility function to convert to the native size in these
	// cases, and then we can CopyOut.
	Native(val uintptr) marshal.Marshallable

	// Value converts a native type back to a generic value.
	// Once a value has been converted to native via the above call -- it
	// can be converted back here.
	Value(val marshal.Marshallable) uintptr

	// Width returns the number of bytes for a native value.
	Width() uint

	// Fork creates a clone of the context.
	Fork() *Context64

	// SyscallNo returns the syscall number.
	SyscallNo() uintptr

	// SyscallSaveOrig save orignal register value.
	SyscallSaveOrig()

	// SyscallArgs returns the syscall arguments in an array.
	SyscallArgs() SyscallArguments

	// Return returns the return value for a system call.
	Return() uintptr

	// SetReturn sets the return value for a system call.
	SetReturn(value uintptr)

	// RestartSyscall reverses over the current syscall instruction, such that
	// when the application resumes execution the syscall will be re-attempted.
	RestartSyscall()

	// RestartSyscallWithRestartBlock reverses over the current syscall
	// instraction and overwrites the current syscall number with that of
	// restart_syscall(2). This causes the application to restart the current
	// syscall with a custom function when execution resumes.
	RestartSyscallWithRestartBlock()

	// IP returns the current instruction pointer.
	IP() uintptr

	// SetIP sets the current instruction pointer.
	SetIP(value uintptr)

	// Stack returns the current stack pointer.
	Stack() uintptr

	// SetStack sets the current stack pointer.
	SetStack(value uintptr)

	// TLS returns the current TLS pointer.
	TLS() uintptr

	// SetTLS sets the current TLS pointer. Returns false if value is invalid.
	SetTLS(value uintptr) bool

	// SetOldRSeqInterruptedIP sets the register that contains the old IP
	// when an "old rseq" restartable sequence is interrupted.
	SetOldRSeqInterruptedIP(value uintptr)

	// StateData returns a pointer to underlying architecture state.
	StateData() *State

	// RegisterMap returns a map of all registers.
	RegisterMap() (map[string]uintptr, error)

	// SignalSetup modifies the context in preparation for handling the
	// given signal.
	//
	// st is the stack where the signal handler frame should be
	// constructed.
	//
	// act is the SigAction that specifies how this signal is being
	// handled.
	//
	// info is the SignalInfo of the signal being delivered.
	//
	// alt is the alternate signal stack (even if the alternate signal
	// stack is not going to be used).
	//
	// sigset is the signal mask before entering the signal handler.
	//
	// featureSet is the application CPU feature set.
	SignalSetup(st *Stack, act *linux.SigAction, info *linux.SignalInfo, alt *linux.SignalStack, sigset linux.SignalSet, featureSet cpuid.FeatureSet) error

	// SignalRestore restores context after returning from a signal
	// handler.
	//
	// st is the current thread stack.
	//
	// rt is true if SignalRestore is being entered from rt_sigreturn and
	// false if SignalRestore is being entered from sigreturn.
	//
	// featureSet is the application CPU feature set.
	//
	// SignalRestore returns the thread's new signal mask.
	SignalRestore(st *Stack, rt bool, featureSet cpuid.FeatureSet) (linux.SignalSet, linux.SignalStack, error)

	// SingleStep returns true if single stepping is enabled.
	SingleStep() bool

	// SetSingleStep enables single stepping.
	SetSingleStep()

	// ClearSingleStep disables single stepping.
	ClearSingleStep()

	// FloatingPointData will be passed to underlying save routines.
	FloatingPointData() *fpu.State

	// NewMmapLayout returns a layout for a new MM, where MinAddr for the
	// returned layout must be no lower than min, and MaxAddr for the returned
	// layout must be no higher than max. Repeated calls to NewMmapLayout may
	// return different layouts.
	NewMmapLayout(min, max hostarch.Addr, limits *limits.LimitSet) (MmapLayout, error)

	// PIELoadAddress returns a preferred load address for a
	// position-independent executable within l.
	PIELoadAddress(l MmapLayout) hostarch.Addr

	// Hack around our package dependences being too broken to support the
	// equivalent of arch_ptrace():

	// PtracePeekUser implements ptrace(PTRACE_PEEKUSR).
	PtracePeekUser(addr uintptr) (marshal.Marshallable, error)

	// PtracePokeUser implements ptrace(PTRACE_POKEUSR).
	PtracePokeUser(addr, data uintptr) error

	// PtraceGetRegs implements ptrace(PTRACE_GETREGS) by writing the
	// general-purpose registers represented by this Context to dst and
	// returning the number of bytes written.
	PtraceGetRegs(dst io.Writer) (int, error)

	// PtraceSetRegs implements ptrace(PTRACE_SETREGS) by reading
	// general-purpose registers from src into this Context and returning the
	// number of bytes read.
	PtraceSetRegs(src io.Reader) (int, error)

	// PtraceGetRegSet implements ptrace(PTRACE_GETREGSET) by writing the
	// register set given by architecture-defined value regset from this
	// Context to dst and returning the number of bytes written, which must be
	// less than or equal to maxlen.
	PtraceGetRegSet(regset uintptr, dst io.Writer, maxlen int, fs cpuid.FeatureSet) (int, error)

	// PtraceSetRegSet implements ptrace(PTRACE_SETREGSET) by reading the
	// register set given by architecture-defined value regset from src and
	// returning the number of bytes read, which must be less than or equal to
	// maxlen.
	PtraceSetRegSet(regset uintptr, src io.Reader, maxlen int, fs cpuid.FeatureSet) (int, error)

	// FullRestore returns 'true' if all CPU registers must be restored
	// when switching to the untrusted application. Typically a task enters
	// and leaves the kernel via a system call. Platform.Switch() may
	// optimize for this by not saving/restoring all registers if allowed
	// by the ABI. For e.g. the amd64 ABI specifies that syscall clobbers
	// %rcx and %r11. If FullRestore returns true then these optimizations
	// must be disabled and all registers restored.
	FullRestore() bool
}

// Compile-time assertion that Context64 implements contextInterface.
var _ = (contextInterface)((*Context64)(nil))

// MmapDirection is a search direction for mmaps.
type MmapDirection int

const (
	// MmapBottomUp instructs mmap to prefer lower addresses.
	MmapBottomUp MmapDirection = iota

	// MmapTopDown instructs mmap to prefer higher addresses.
	MmapTopDown
)

// MmapLayout defines the layout of the user address space for a particular
// MemoryManager.
//
// Note that "highest address" below is always exclusive.
//
// +stateify savable
type MmapLayout struct {
	// MinAddr is the lowest mappable address.
	MinAddr hostarch.Addr

	// MaxAddr is the highest mappable address.
	MaxAddr hostarch.Addr

	// BottomUpBase is the lowest address that may be returned for a
	// MmapBottomUp mmap.
	BottomUpBase hostarch.Addr

	// TopDownBase is the highest address that may be returned for a
	// MmapTopDown mmap.
	TopDownBase hostarch.Addr

	// DefaultDirection is the direction for most non-fixed mmaps in this
	// layout.
	DefaultDirection MmapDirection

	// MaxStackRand is the maximum randomization to apply to stack
	// allocations to maintain a proper gap between the stack and
	// TopDownBase.
	MaxStackRand uint64
}

// Valid returns true if this layout is valid.
func (m *MmapLayout) Valid() bool {
	if m.MinAddr > m.MaxAddr {
		return false
	}
	if m.BottomUpBase < m.MinAddr {
		return false
	}
	if m.BottomUpBase > m.MaxAddr {
		return false
	}
	if m.TopDownBase < m.MinAddr {
		return false
	}
	if m.TopDownBase > m.MaxAddr {
		return false
	}
	return true
}

// SyscallArgument is an argument supplied to a syscall implementation. The
// methods used to access the arguments are named after the ***C type name*** and
// they convert to the closest Go type available. For example, Int() refers to a
// 32-bit signed integer argument represented in Go as an int32.
//
// Using the accessor methods guarantees that the conversion between types is
// correct, taking into account size and signedness (i.e., zero-extension vs
// signed-extension).
type SyscallArgument struct {
	// Prefer to use accessor methods instead of 'Value' directly.
	Value uintptr
}

// SyscallArguments represents the set of arguments passed to a syscall.
type SyscallArguments [6]SyscallArgument

// Pointer returns the hostarch.Addr representation of a pointer argument.
func (a SyscallArgument) Pointer() hostarch.Addr {
	return hostarch.Addr(a.Value)
}

// Int returns the int32 representation of a 32-bit signed integer argument.
func (a SyscallArgument) Int() int32 {
	return int32(a.Value)
}

// Uint returns the uint32 representation of a 32-bit unsigned integer argument.
func (a SyscallArgument) Uint() uint32 {
	return uint32(a.Value)
}

// Int64 returns the int64 representation of a 64-bit signed integer argument.
func (a SyscallArgument) Int64() int64 {
	return int64(a.Value)
}

// Uint64 returns the uint64 representation of a 64-bit unsigned integer argument.
func (a SyscallArgument) Uint64() uint64 {
	return uint64(a.Value)
}

// SizeT returns the uint representation of a size_t argument.
func (a SyscallArgument) SizeT() uint {
	return uint(a.Value)
}

// ModeT returns the int representation of a mode_t argument.
func (a SyscallArgument) ModeT() uint {
	return uint(uint16(a.Value))
}
