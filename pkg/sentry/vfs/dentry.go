// Copyright 2019 The gVisor Authors.
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

package vfs

import (
	"github.com/talismancer/gvisor-ligolo/pkg/atomicbitops"
	"github.com/talismancer/gvisor-ligolo/pkg/context"
	"github.com/talismancer/gvisor-ligolo/pkg/errors/linuxerr"
	"github.com/talismancer/gvisor-ligolo/pkg/refs"
	"github.com/talismancer/gvisor-ligolo/pkg/sync"
)

// Dentry represents a node in a Filesystem tree at which a file exists.
//
// Dentries are reference-counted. Unless otherwise specified, all Dentry
// methods require that a reference is held.
//
// Dentry is loosely analogous to Linux's struct dentry, but:
//
//   - VFS does not associate Dentries with inodes. gVisor interacts primarily
//     with filesystems that are accessed through filesystem APIs (as opposed to
//     raw block devices); many such APIs support only paths and file descriptors,
//     and not inodes. Furthermore, when parties outside the scope of VFS can
//     rename inodes on such filesystems, VFS generally cannot "follow" the rename,
//     both due to synchronization issues and because it may not even be able to
//     name the destination path; this implies that it would in fact be incorrect
//     for Dentries to be associated with inodes on such filesystems. Consequently,
//     operations that are inode operations in Linux are FilesystemImpl methods
//     and/or FileDescriptionImpl methods in gVisor's VFS. Filesystems that do
//     support inodes may store appropriate state in implementations of DentryImpl.
//
//   - VFS does not require that Dentries are instantiated for all paths accessed
//     through VFS, only those that are tracked beyond the scope of a single
//     Filesystem operation. This includes file descriptions, mount points, mount
//     roots, process working directories, and chroots. This avoids instantiation
//     of Dentries for operations on mutable remote filesystems that can't actually
//     cache any state in the Dentry.
//
//   - VFS does not track filesystem structure (i.e. relationships between
//     Dentries), since both the relevant state and synchronization are
//     filesystem-specific.
//
//   - For the reasons above, VFS is not directly responsible for managing Dentry
//     lifetime. Dentry reference counts only indicate the extent to which VFS
//     requires Dentries to exist; Filesystems may elect to cache or discard
//     Dentries with zero references.
//
// +stateify savable
type Dentry struct {
	// mu synchronizes deletion/invalidation and mounting over this Dentry.
	mu sync.Mutex `state:"nosave"`

	// dead is true if the file represented by this Dentry has been deleted (by
	// CommitDeleteDentry or CommitRenameReplaceDentry) or invalidated (by
	// InvalidateDentry). dead is protected by mu.
	dead bool

	// evictable is set by the VFS layer or filesystems like overlayfs as a hint
	// that this dentry will not be accessed hence forth. So filesystems that
	// cache dentries locally can use this hint to release the dentry when all
	// references are dropped. evictable is protected by mu.
	evictable bool

	// mounts is the number of Mounts for which this Dentry is Mount.point.
	mounts atomicbitops.Uint32

	// impl is the DentryImpl associated with this Dentry. impl is immutable.
	// This should be the last field in Dentry.
	impl DentryImpl
}

// Init must be called before first use of d.
func (d *Dentry) Init(impl DentryImpl) {
	d.impl = impl
}

// Impl returns the DentryImpl associated with d.
func (d *Dentry) Impl() DentryImpl {
	return d.impl
}

// DentryImpl contains implementation details for a Dentry. Implementations of
// DentryImpl should contain their associated Dentry by value as their first
// field.
//
// +stateify savable
type DentryImpl interface {
	// IncRef increments the Dentry's reference count. A Dentry with a non-zero
	// reference count must remain coherent with the state of the filesystem.
	IncRef()

	// TryIncRef increments the Dentry's reference count and returns true. If
	// the Dentry's reference count is zero, TryIncRef may do nothing and
	// return false. (It is also permitted to succeed if it can restore the
	// guarantee that the Dentry is coherent with the state of the filesystem.)
	//
	// TryIncRef does not require that a reference is held on the Dentry.
	TryIncRef() bool

	// DecRef decrements the Dentry's reference count.
	DecRef(ctx context.Context)

	// InotifyWithParent notifies all watches on the targets represented by this
	// dentry and its parent. The parent's watches are notified first, followed
	// by this dentry's.
	//
	// InotifyWithParent automatically adds the IN_ISDIR flag for dentries
	// representing directories.
	//
	// Note that the events may not actually propagate up to the user, depending
	// on the event masks.
	InotifyWithParent(ctx context.Context, events, cookie uint32, et EventType)

	// Watches returns the set of inotify watches for the file corresponding to
	// the Dentry. Dentries that are hard links to the same underlying file
	// share the same watches.
	//
	// The caller does not need to hold a reference on the dentry.
	Watches() *Watches

	// OnZeroWatches is called whenever the number of watches on a dentry drops
	// to zero. This is needed by some FilesystemImpls (e.g. gofer) to manage
	// dentry lifetime.
	//
	// The caller does not need to hold a reference on the dentry. OnZeroWatches
	// may acquire inotify locks, so to prevent deadlock, no inotify locks should
	// be held by the caller.
	OnZeroWatches(ctx context.Context)
}

// IncRef increments d's reference count.
func (d *Dentry) IncRef() {
	d.impl.IncRef()
}

// TryIncRef increments d's reference count and returns true. If d's reference
// count is zero, TryIncRef may instead do nothing and return false.
func (d *Dentry) TryIncRef() bool {
	return d.impl.TryIncRef()
}

// DecRef decrements d's reference count.
func (d *Dentry) DecRef(ctx context.Context) {
	d.impl.DecRef(ctx)
}

// IsDead returns true if d has been deleted or invalidated by its owning
// filesystem.
func (d *Dentry) IsDead() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dead
}

// IsEvictable returns true if d is evictable from filesystem dentry cache.
func (d *Dentry) IsEvictable() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.evictable
}

// MarkEvictable marks d as evictable.
func (d *Dentry) MarkEvictable() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.evictable = true
}

func (d *Dentry) isMounted() bool {
	return d.mounts.Load() != 0
}

// InotifyWithParent notifies all watches on the targets represented by d and
// its parent of events.
func (d *Dentry) InotifyWithParent(ctx context.Context, events, cookie uint32, et EventType) {
	d.impl.InotifyWithParent(ctx, events, cookie, et)
}

// Watches returns the set of inotify watches associated with d.
func (d *Dentry) Watches() *Watches {
	return d.impl.Watches()
}

// OnZeroWatches performs cleanup tasks whenever the number of watches on a
// dentry drops to zero.
func (d *Dentry) OnZeroWatches(ctx context.Context) {
	d.impl.OnZeroWatches(ctx)
}

// The following functions are exported so that filesystem implementations can
// use them. The vfs package, and users of VFS, should not call these
// functions.

// PrepareDeleteDentry must be called before attempting to delete the file
// represented by d. If PrepareDeleteDentry succeeds, the caller must call
// AbortDeleteDentry or CommitDeleteDentry depending on the deletion's outcome.
// +checklocksacquire:d.mu
func (vfs *VirtualFilesystem) PrepareDeleteDentry(mntns *MountNamespace, d *Dentry) error {
	vfs.mountMu.Lock()
	if mntns.mountpoints[d] != 0 {
		vfs.mountMu.Unlock()
		return linuxerr.EBUSY // +checklocksforce: inconsistent return.
	}
	d.mu.Lock()
	vfs.mountMu.Unlock()
	// Return with d.mu locked to block attempts to mount over it; it will be
	// unlocked by AbortDeleteDentry or CommitDeleteDentry.
	return nil
}

// AbortDeleteDentry must be called after PrepareDeleteDentry if the deletion
// fails.
// +checklocksrelease:d.mu
func (vfs *VirtualFilesystem) AbortDeleteDentry(d *Dentry) {
	d.mu.Unlock()
}

// CommitDeleteDentry must be called after PrepareDeleteDentry if the deletion
// succeeds.
// +checklocksrelease:d.mu
func (vfs *VirtualFilesystem) CommitDeleteDentry(ctx context.Context, d *Dentry) {
	d.dead = true
	d.mu.Unlock()
	if d.isMounted() {
		vfs.forgetDeadMountpoint(ctx, d, false /*skipDecRef*/)
	}
}

// InvalidateDentry is called when d ceases to represent the file it formerly
// did for reasons outside of VFS' control (e.g. d represents the local state
// of a file on a remote filesystem on which the file has already been
// deleted). If d is mounted, the method returns a list of Virtual Dentries
// mounted on d that the caller is responsible for DecRefing.
func (vfs *VirtualFilesystem) InvalidateDentry(ctx context.Context, d *Dentry) []refs.RefCounter {
	d.mu.Lock()
	d.dead = true
	d.mu.Unlock()
	if d.isMounted() {
		return vfs.forgetDeadMountpoint(ctx, d, true /*skipDecRef*/)
	}
	return nil
}

// PrepareRenameDentry must be called before attempting to rename the file
// represented by from. If to is not nil, it represents the file that will be
// replaced or exchanged by the rename. If PrepareRenameDentry succeeds, the
// caller must call AbortRenameDentry, CommitRenameReplaceDentry, or
// CommitRenameExchangeDentry depending on the rename's outcome.
//
// Preconditions:
//   - If to is not nil, it must be a child Dentry from the same Filesystem.
//   - from != to.
//
// +checklocksacquire:from.mu
// +checklocksacquire:to.mu
func (vfs *VirtualFilesystem) PrepareRenameDentry(mntns *MountNamespace, from, to *Dentry) error {
	vfs.mountMu.Lock()
	if mntns.mountpoints[from] != 0 {
		vfs.mountMu.Unlock()
		return linuxerr.EBUSY // +checklocksforce: no locks acquired.
	}
	if to != nil {
		if mntns.mountpoints[to] != 0 {
			vfs.mountMu.Unlock()
			return linuxerr.EBUSY // +checklocksforce: no locks acquired.
		}
		to.mu.Lock()
	}
	from.mu.Lock()
	vfs.mountMu.Unlock()
	// Return with from.mu and to.mu locked, which will be unlocked by
	// AbortRenameDentry, CommitRenameReplaceDentry, or
	// CommitRenameExchangeDentry.
	return nil // +checklocksforce: to may not be acquired.
}

// AbortRenameDentry must be called after PrepareRenameDentry if the rename
// fails.
// +checklocksrelease:from.mu
// +checklocksrelease:to.mu
func (vfs *VirtualFilesystem) AbortRenameDentry(from, to *Dentry) {
	from.mu.Unlock()
	if to != nil {
		to.mu.Unlock()
	}
}

// CommitRenameReplaceDentry must be called after the file represented by from
// is renamed without RENAME_EXCHANGE. If to is not nil, it represents the file
// that was replaced by from.
//
// Preconditions: PrepareRenameDentry was previously called on from and to.
// +checklocksrelease:from.mu
// +checklocksrelease:to.mu
func (vfs *VirtualFilesystem) CommitRenameReplaceDentry(ctx context.Context, from, to *Dentry) {
	from.mu.Unlock()
	if to != nil {
		to.dead = true
		to.mu.Unlock()
		if to.isMounted() {
			vfs.forgetDeadMountpoint(ctx, to, false /*skipDecRef*/)
		}
	}
}

// CommitRenameExchangeDentry must be called after the files represented by
// from and to are exchanged by rename(RENAME_EXCHANGE).
//
// Preconditions: PrepareRenameDentry was previously called on from and to.
// +checklocksrelease:from.mu
// +checklocksrelease:to.mu
func (vfs *VirtualFilesystem) CommitRenameExchangeDentry(from, to *Dentry) {
	from.mu.Unlock()
	to.mu.Unlock()
}

// forgetDeadMountpoint is called when a mount point is deleted or invalidated
// to umount all mounts using it in all other mount namespaces. If skipDecRef
// is true, the method returns a list of reference counted objects with an
// an extra reference.
//
// forgetDeadMountpoint is analogous to Linux's
// fs/namespace.c:__detach_mounts().
func (vfs *VirtualFilesystem) forgetDeadMountpoint(ctx context.Context, d *Dentry, skipDecRef bool) []refs.RefCounter {
	var (
		vdsToDecRef    []VirtualDentry
		mountsToDecRef []*Mount
	)
	vfs.mountMu.Lock()
	vfs.mounts.seq.BeginWrite()
	for mnt := range vfs.mountpoints[d] {
		vdsToDecRef, mountsToDecRef = vfs.umountRecursiveLocked(mnt, &umountRecursiveOptions{}, vdsToDecRef, mountsToDecRef)
	}
	vfs.mounts.seq.EndWrite()
	vfs.mountMu.Unlock()
	rcs := make([]refs.RefCounter, 0, len(vdsToDecRef)+len(mountsToDecRef))
	for _, vd := range vdsToDecRef {
		rcs = append(rcs, vd)
	}
	for _, mnt := range mountsToDecRef {
		rcs = append(rcs, mnt)
	}
	if skipDecRef {
		return rcs
	}
	for _, rc := range rcs {
		rc.DecRef(ctx)
	}
	return nil
}
