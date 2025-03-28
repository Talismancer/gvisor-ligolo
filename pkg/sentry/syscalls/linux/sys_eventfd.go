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
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/arch"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/fsimpl/eventfd"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/kernel"
)

// Eventfd2 implements linux syscall eventfd2(2).
func Eventfd2(t *kernel.Task, sysno uintptr, args arch.SyscallArguments) (uintptr, *kernel.SyscallControl, error) {
	initVal := uint64(args[0].Uint())
	flags := uint(args[1].Uint())
	allOps := uint(linux.EFD_SEMAPHORE | linux.EFD_NONBLOCK | linux.EFD_CLOEXEC)

	if flags & ^allOps != 0 {
		return 0, nil, linuxerr.EINVAL
	}

	vfsObj := t.Kernel().VFS()
	fileFlags := uint32(linux.O_RDWR)
	if flags&linux.EFD_NONBLOCK != 0 {
		fileFlags |= linux.O_NONBLOCK
	}
	semMode := flags&linux.EFD_SEMAPHORE != 0
	eventfd, err := eventfd.New(t, vfsObj, initVal, semMode, fileFlags)
	if err != nil {
		return 0, nil, err
	}
	defer eventfd.DecRef(t)

	fd, err := t.NewFDFrom(0, eventfd, kernel.FDFlags{
		CloseOnExec: flags&linux.EFD_CLOEXEC != 0,
	})
	if err != nil {
		return 0, nil, err
	}

	return uintptr(fd), nil, nil
}

// Eventfd implements linux syscall eventfd(2).
func Eventfd(t *kernel.Task, sysno uintptr, args arch.SyscallArguments) (uintptr, *kernel.SyscallControl, error) {
	args[1].Value = 0
	return Eventfd2(t, sysno, args)
}
