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

package linux

import (
	"github.com/talismancer/gvisor-ligolo/pkg/abi/linux"
	"github.com/talismancer/gvisor-ligolo/pkg/errors/linuxerr"
	"github.com/talismancer/gvisor-ligolo/pkg/fspath"
	"github.com/talismancer/gvisor-ligolo/pkg/hostarch"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/kernel"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/vfs"
)

func copyInPath(t *kernel.Task, addr hostarch.Addr) (fspath.Path, error) {
	pathname, err := t.CopyInString(addr, linux.PATH_MAX)
	if err != nil {
		return fspath.Path{}, err
	}
	return fspath.Parse(pathname), nil
}

type taskPathOperation struct {
	pop          vfs.PathOperation
	haveStartRef bool
}

func getTaskPathOperation(t *kernel.Task, dirfd int32, path fspath.Path, shouldAllowEmptyPath shouldAllowEmptyPath, shouldFollowFinalSymlink shouldFollowFinalSymlink) (taskPathOperation, error) {
	root := t.FSContext().RootDirectory()
	start := root
	haveStartRef := false
	if !path.Absolute {
		if !path.HasComponents() && !bool(shouldAllowEmptyPath) {
			root.DecRef(t)
			return taskPathOperation{}, linuxerr.ENOENT
		}
		if dirfd == linux.AT_FDCWD {
			start = t.FSContext().WorkingDirectory()
			haveStartRef = true
		} else {
			dirfile := t.GetFile(dirfd)
			if dirfile == nil {
				root.DecRef(t)
				return taskPathOperation{}, linuxerr.EBADF
			}
			start = dirfile.VirtualDentry()
			start.IncRef()
			haveStartRef = true
			dirfile.DecRef(t)
		}
	}
	return taskPathOperation{
		pop: vfs.PathOperation{
			Root:               root,
			Start:              start,
			Path:               path,
			FollowFinalSymlink: bool(shouldFollowFinalSymlink),
		},
		haveStartRef: haveStartRef,
	}, nil
}

func (tpop *taskPathOperation) Release(t *kernel.Task) {
	tpop.pop.Root.DecRef(t)
	if tpop.haveStartRef {
		tpop.pop.Start.DecRef(t)
		tpop.haveStartRef = false
	}
}

type shouldAllowEmptyPath bool

const (
	disallowEmptyPath shouldAllowEmptyPath = false
	allowEmptyPath    shouldAllowEmptyPath = true
)

type shouldFollowFinalSymlink bool

const (
	nofollowFinalSymlink shouldFollowFinalSymlink = false
	followFinalSymlink   shouldFollowFinalSymlink = true
)
