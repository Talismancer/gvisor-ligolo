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
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/kernel"
)

const (
	onlyScheduler = linux.SCHED_NORMAL
	onlyPriority  = 0
)

// SchedParam replicates struct sched_param in sched.h.
//
// +marshal
type SchedParam struct {
	schedPriority int32
}

// SchedGetparam implements linux syscall sched_getparam(2).
func SchedGetparam(t *kernel.Task, sysno uintptr, args arch.SyscallArguments) (uintptr, *kernel.SyscallControl, error) {
	pid := args[0].Int()
	param := args[1].Pointer()
	if param == 0 {
		return 0, nil, linuxerr.EINVAL
	}
	if pid < 0 {
		return 0, nil, linuxerr.EINVAL
	}
	if pid != 0 && t.PIDNamespace().TaskWithID(kernel.ThreadID(pid)) == nil {
		return 0, nil, linuxerr.ESRCH
	}
	r := SchedParam{schedPriority: onlyPriority}
	if _, err := r.CopyOut(t, param); err != nil {
		return 0, nil, err
	}

	return 0, nil, nil
}

// SchedGetscheduler implements linux syscall sched_getscheduler(2).
func SchedGetscheduler(t *kernel.Task, sysno uintptr, args arch.SyscallArguments) (uintptr, *kernel.SyscallControl, error) {
	pid := args[0].Int()
	if pid < 0 {
		return 0, nil, linuxerr.EINVAL
	}
	if pid != 0 && t.PIDNamespace().TaskWithID(kernel.ThreadID(pid)) == nil {
		return 0, nil, linuxerr.ESRCH
	}
	return onlyScheduler, nil, nil
}

// SchedSetscheduler implements linux syscall sched_setscheduler(2).
func SchedSetscheduler(t *kernel.Task, sysno uintptr, args arch.SyscallArguments) (uintptr, *kernel.SyscallControl, error) {
	pid := args[0].Int()
	policy := args[1].Int()
	param := args[2].Pointer()
	if pid < 0 {
		return 0, nil, linuxerr.EINVAL
	}
	if policy != onlyScheduler {
		return 0, nil, linuxerr.EINVAL
	}
	if pid != 0 && t.PIDNamespace().TaskWithID(kernel.ThreadID(pid)) == nil {
		return 0, nil, linuxerr.ESRCH
	}
	var r SchedParam
	if _, err := r.CopyIn(t, param); err != nil {
		return 0, nil, linuxerr.EINVAL
	}
	if r.schedPriority != onlyPriority {
		return 0, nil, linuxerr.EINVAL
	}
	return 0, nil, nil
}

// SchedGetPriorityMax implements linux syscall sched_get_priority_max(2).
func SchedGetPriorityMax(t *kernel.Task, sysno uintptr, args arch.SyscallArguments) (uintptr, *kernel.SyscallControl, error) {
	return onlyPriority, nil, nil
}

// SchedGetPriorityMin implements linux syscall sched_get_priority_min(2).
func SchedGetPriorityMin(t *kernel.Task, sysno uintptr, args arch.SyscallArguments) (uintptr, *kernel.SyscallControl, error) {
	return onlyPriority, nil, nil
}
