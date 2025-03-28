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

package fuse

import (
	"github.com/talismancer/gvisor-ligolo/pkg/abi/linux"
	"github.com/talismancer/gvisor-ligolo/pkg/context"
	"github.com/talismancer/gvisor-ligolo/pkg/errors/linuxerr"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/kernel/auth"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/vfs"
	"github.com/talismancer/gvisor-ligolo/pkg/usermem"
)

type directoryFD struct {
	fileDescription
}

// Allocate implements directoryFD.Allocate.
func (*directoryFD) Allocate(ctx context.Context, mode, offset, length uint64) error {
	return linuxerr.EISDIR
}

// PRead implements vfs.FileDescriptionImpl.PRead.
func (*directoryFD) PRead(ctx context.Context, dst usermem.IOSequence, offset int64, opts vfs.ReadOptions) (int64, error) {
	return 0, linuxerr.EISDIR
}

// Read implements vfs.FileDescriptionImpl.Read.
func (*directoryFD) Read(ctx context.Context, dst usermem.IOSequence, opts vfs.ReadOptions) (int64, error) {
	return 0, linuxerr.EISDIR
}

// PWrite implements vfs.FileDescriptionImpl.PWrite.
func (*directoryFD) PWrite(ctx context.Context, src usermem.IOSequence, offset int64, opts vfs.WriteOptions) (int64, error) {
	return 0, linuxerr.EISDIR
}

// Write implements vfs.FileDescriptionImpl.Write.
func (*directoryFD) Write(ctx context.Context, src usermem.IOSequence, opts vfs.WriteOptions) (int64, error) {
	return 0, linuxerr.EISDIR
}

// IterDirents implements vfs.FileDescriptionImpl.IterDirents.
func (dir *directoryFD) IterDirents(ctx context.Context, callback vfs.IterDirentsCallback) error {
	fusefs := dir.inode().fs

	in := linux.FUSEReadIn{
		Fh:     dir.Fh,
		Offset: uint64(dir.off.Load()),
		Size:   linux.FUSE_PAGE_SIZE,
		Flags:  dir.statusFlags(),
	}

	// TODO(gVisor.dev/issue/3404): Support FUSE_READDIRPLUS.
	req := fusefs.conn.NewRequest(auth.CredentialsFromContext(ctx), pidFromContext(ctx), dir.inode().nodeID, linux.FUSE_READDIR, &in)
	res, err := fusefs.conn.Call(ctx, req)
	if err != nil {
		return err
	}
	if err := res.Error(); err != nil {
		return err
	}

	var out linux.FUSEDirents
	if err := res.UnmarshalPayload(&out); err != nil {
		return err
	}

	for _, fuseDirent := range out.Dirents {
		nextOff := int64(fuseDirent.Meta.Off)
		dirent := vfs.Dirent{
			Name:    fuseDirent.Name,
			Type:    uint8(fuseDirent.Meta.Type),
			Ino:     fuseDirent.Meta.Ino,
			NextOff: nextOff,
		}

		if err := callback.Handle(dirent); err != nil {
			return err
		}
		dir.off.Store(nextOff)
	}

	return nil
}
