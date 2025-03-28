// Copyright 2020 The gVisor Authors.
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

//go:build go1.1
// +build go1.1

package filter

import (
	"github.com/talismancer/gvisor-ligolo/pkg/seccomp"
	"golang.org/x/sys/unix"
)

// profileFilters returns extra syscalls made by runtime/pprof package.
func profileFilters() seccomp.SyscallRules {
	return seccomp.SyscallRules{
		unix.SYS_OPENAT: []seccomp.Rule{
			{
				seccomp.MatchAny{},
				seccomp.MatchAny{},
				seccomp.EqualTo(unix.O_RDONLY | unix.O_LARGEFILE | unix.O_CLOEXEC),
			},
		},
	}
}
