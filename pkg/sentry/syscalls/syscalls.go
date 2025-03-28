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

// Package syscalls is the interface from the application to the kernel.
// Traditionally, syscalls is the interface that is used by applications to
// request services from the kernel of a operating system. We provide a
// user-mode kernel that needs to handle those requests coming from unmodified
// applications. Therefore, we still use the term "syscalls" to denote this
// interface.
//
// Note that the stubs in this package may merely provide the interface, not
// the actual implementation. It just makes writing syscall stubs
// straightforward.
package syscalls

import (
	"fmt"

	"github.com/talismancer/gvisor-ligolo/pkg/abi/linux"
	"github.com/talismancer/gvisor-ligolo/pkg/errors/linuxerr"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/arch"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/kernel"
)

// Supported returns a syscall that is fully supported.
func Supported(name string, fn kernel.SyscallFn) kernel.Syscall {
	return kernel.Syscall{
		Name:         name,
		Fn:           fn,
		SupportLevel: kernel.SupportFull,
		Note:         "Fully Supported.",
	}
}

// SupportedPoint returns a syscall that is fully supported with a correspoding
// seccheck.Point.
func SupportedPoint(name string, fn kernel.SyscallFn, cb kernel.SyscallToProto) kernel.Syscall {
	sys := Supported(name, fn)
	sys.PointCallback = cb
	return sys
}

// PartiallySupported returns a syscall that has a partial implementation.
func PartiallySupported(name string, fn kernel.SyscallFn, note string, urls []string) kernel.Syscall {
	return kernel.Syscall{
		Name:         name,
		Fn:           fn,
		SupportLevel: kernel.SupportPartial,
		Note:         note,
		URLs:         urls,
	}
}

// PartiallySupportedPoint returns a syscall that has a partial implementation
// with a correspoding seccheck.Point.
func PartiallySupportedPoint(name string, fn kernel.SyscallFn, cb kernel.SyscallToProto, note string, urls []string) kernel.Syscall {
	sys := PartiallySupported(name, fn, note, urls)
	sys.PointCallback = cb
	return sys
}

// Error returns a syscall handler that will always give the passed error.
func Error(name string, err error, note string, urls []string) kernel.Syscall {
	if note != "" {
		note = note + "; "
	}
	return kernel.Syscall{
		Name: name,
		Fn: func(t *kernel.Task, sysno uintptr, args arch.SyscallArguments) (uintptr, *kernel.SyscallControl, error) {
			kernel.IncrementUnimplementedSyscallCounter(sysno)
			return 0, nil, err
		},
		SupportLevel: kernel.SupportUnimplemented,
		Note:         fmt.Sprintf("%sReturns %q.", note, err.Error()),
		URLs:         urls,
	}
}

// ErrorWithEvent gives a syscall function that sends an unimplemented
// syscall event via the event channel and returns the passed error.
func ErrorWithEvent(name string, err error, note string, urls []string) kernel.Syscall {
	if note != "" {
		note = note + "; "
	}
	return kernel.Syscall{
		Name: name,
		Fn: func(t *kernel.Task, sysno uintptr, args arch.SyscallArguments) (uintptr, *kernel.SyscallControl, error) {
			t.Kernel().EmitUnimplementedEvent(t, sysno)
			return 0, nil, err
		},
		SupportLevel: kernel.SupportUnimplemented,
		Note:         fmt.Sprintf("%sReturns %q.", note, err.Error()),
		URLs:         urls,
	}
}

// CapError gives a syscall function that checks for capability c.  If the task
// has the capability, it returns ENOSYS, otherwise EPERM. To unprivileged
// tasks, it will seem like there is an implementation.
func CapError(name string, c linux.Capability, note string, urls []string) kernel.Syscall {
	if note != "" {
		note = note + "; "
	}
	return kernel.Syscall{
		Name: name,
		Fn: func(t *kernel.Task, sysno uintptr, args arch.SyscallArguments) (uintptr, *kernel.SyscallControl, error) {
			if !t.HasCapability(c) {
				return 0, nil, linuxerr.EPERM
			}
			t.Kernel().EmitUnimplementedEvent(t, sysno)
			return 0, nil, linuxerr.ENOSYS
		},
		SupportLevel: kernel.SupportUnimplemented,
		Note:         fmt.Sprintf("%sReturns %q if the process does not have %s; %q otherwise.", note, linuxerr.EPERM, c.String(), linuxerr.ENOSYS),
		URLs:         urls,
	}
}
