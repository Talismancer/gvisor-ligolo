// Copyright 2023 The gVisor Authors.
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

package nvproxy

import (
	"github.com/talismancer/gvisor-ligolo/pkg/context"
	"github.com/talismancer/gvisor-ligolo/pkg/errors/linuxerr"
	"github.com/talismancer/gvisor-ligolo/pkg/hostarch"
	"github.com/talismancer/gvisor-ligolo/pkg/log"
	"github.com/talismancer/gvisor-ligolo/pkg/safemem"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/memmap"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/vfs"
)

// ConfigureMMap implements vfs.FileDescriptionImpl.ConfigureMMap.
func (fd *uvmFD) ConfigureMMap(ctx context.Context, opts *memmap.MMapOpts) error {
	return vfs.GenericConfigureMMap(&fd.vfsfd, fd, opts)
}

// AddMapping implements memmap.Mappable.AddMapping.
func (fd *uvmFD) AddMapping(ctx context.Context, ms memmap.MappingSpace, ar hostarch.AddrRange, offset uint64, writable bool) error {
	return nil
}

// RemoveMapping implements memmap.Mappable.RemoveMapping.
func (fd *uvmFD) RemoveMapping(ctx context.Context, ms memmap.MappingSpace, ar hostarch.AddrRange, offset uint64, writable bool) {
}

// CopyMapping implements memmap.Mappable.CopyMapping.
func (fd *uvmFD) CopyMapping(ctx context.Context, ms memmap.MappingSpace, srcAR, dstAR hostarch.AddrRange, offset uint64, writable bool) error {
	return nil
}

// Translate implements memmap.Mappable.Translate.
func (fd *uvmFD) Translate(ctx context.Context, required, optional memmap.MappableRange, at hostarch.AccessType) ([]memmap.Translation, error) {
	return []memmap.Translation{
		{
			Source: optional,
			File:   &fd.memmapFile,
			Offset: optional.Start,
			// kernel-open/nvidia-uvm/uvm.c:uvm_mmap() requires mappings to be
			// PROT_READ|PROT_WRITE.
			Perms: hostarch.ReadWrite,
		},
	}, nil
}

// InvalidateUnsavable implements memmap.Mappable.InvalidateUnsavable.
func (fd *uvmFD) InvalidateUnsavable(ctx context.Context) error {
	return nil
}

type uvmFDMemmapFile struct {
	fd *uvmFD
}

// IncRef implements memmap.File.IncRef.
func (mf *uvmFDMemmapFile) IncRef(fr memmap.FileRange, memCgID uint32) {
}

// DecRef implements memmap.File.DecRef.
func (mf *uvmFDMemmapFile) DecRef(fr memmap.FileRange) {
}

// MapInternal implements memmap.File.MapInternal.
func (mf *uvmFDMemmapFile) MapInternal(fr memmap.FileRange, at hostarch.AccessType) (safemem.BlockSeq, error) {
	// TODO(jamieliu): make an attempt with MAP_FIXED_NOREPLACE?
	log.Traceback("nvproxy: rejecting uvmFDMemmapFile.MapInternal")
	return safemem.BlockSeq{}, linuxerr.EINVAL
}

// FD implements memmap.File.FD.
func (mf *uvmFDMemmapFile) FD() int {
	return int(mf.fd.hostFD)
}
