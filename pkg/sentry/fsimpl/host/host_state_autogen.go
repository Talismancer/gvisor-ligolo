// automatically generated by stateify.

package host

import (
	"github.com/talismancer/gvisor-ligolo/pkg/state"
)

func (v *virtualOwner) StateTypeName() string {
	return "pkg/sentry/fsimpl/host.virtualOwner"
}

func (v *virtualOwner) StateFields() []string {
	return []string{
		"enabled",
		"uid",
		"gid",
		"mode",
	}
}

func (v *virtualOwner) beforeSave() {}

// +checklocksignore
func (v *virtualOwner) StateSave(stateSinkObject state.Sink) {
	v.beforeSave()
	stateSinkObject.Save(0, &v.enabled)
	stateSinkObject.Save(1, &v.uid)
	stateSinkObject.Save(2, &v.gid)
	stateSinkObject.Save(3, &v.mode)
}

func (v *virtualOwner) afterLoad() {}

// +checklocksignore
func (v *virtualOwner) StateLoad(stateSourceObject state.Source) {
	stateSourceObject.Load(0, &v.enabled)
	stateSourceObject.Load(1, &v.uid)
	stateSourceObject.Load(2, &v.gid)
	stateSourceObject.Load(3, &v.mode)
}

func (i *inode) StateTypeName() string {
	return "pkg/sentry/fsimpl/host.inode"
}

func (i *inode) StateFields() []string {
	return []string{
		"CachedMappable",
		"InodeNoStatFS",
		"InodeAnonymous",
		"InodeNotDirectory",
		"InodeNotSymlink",
		"InodeTemporary",
		"InodeWatches",
		"locks",
		"inodeRefs",
		"hostFD",
		"ino",
		"ftype",
		"epollable",
		"seekable",
		"isTTY",
		"savable",
		"readonly",
		"queue",
		"virtualOwner",
		"haveBuf",
		"buf",
	}
}

// +checklocksignore
func (i *inode) StateSave(stateSinkObject state.Sink) {
	i.beforeSave()
	stateSinkObject.Save(0, &i.CachedMappable)
	stateSinkObject.Save(1, &i.InodeNoStatFS)
	stateSinkObject.Save(2, &i.InodeAnonymous)
	stateSinkObject.Save(3, &i.InodeNotDirectory)
	stateSinkObject.Save(4, &i.InodeNotSymlink)
	stateSinkObject.Save(5, &i.InodeTemporary)
	stateSinkObject.Save(6, &i.InodeWatches)
	stateSinkObject.Save(7, &i.locks)
	stateSinkObject.Save(8, &i.inodeRefs)
	stateSinkObject.Save(9, &i.hostFD)
	stateSinkObject.Save(10, &i.ino)
	stateSinkObject.Save(11, &i.ftype)
	stateSinkObject.Save(12, &i.epollable)
	stateSinkObject.Save(13, &i.seekable)
	stateSinkObject.Save(14, &i.isTTY)
	stateSinkObject.Save(15, &i.savable)
	stateSinkObject.Save(16, &i.readonly)
	stateSinkObject.Save(17, &i.queue)
	stateSinkObject.Save(18, &i.virtualOwner)
	stateSinkObject.Save(19, &i.haveBuf)
	stateSinkObject.Save(20, &i.buf)
}

// +checklocksignore
func (i *inode) StateLoad(stateSourceObject state.Source) {
	stateSourceObject.Load(0, &i.CachedMappable)
	stateSourceObject.Load(1, &i.InodeNoStatFS)
	stateSourceObject.Load(2, &i.InodeAnonymous)
	stateSourceObject.Load(3, &i.InodeNotDirectory)
	stateSourceObject.Load(4, &i.InodeNotSymlink)
	stateSourceObject.Load(5, &i.InodeTemporary)
	stateSourceObject.Load(6, &i.InodeWatches)
	stateSourceObject.Load(7, &i.locks)
	stateSourceObject.Load(8, &i.inodeRefs)
	stateSourceObject.Load(9, &i.hostFD)
	stateSourceObject.Load(10, &i.ino)
	stateSourceObject.Load(11, &i.ftype)
	stateSourceObject.Load(12, &i.epollable)
	stateSourceObject.Load(13, &i.seekable)
	stateSourceObject.Load(14, &i.isTTY)
	stateSourceObject.Load(15, &i.savable)
	stateSourceObject.Load(16, &i.readonly)
	stateSourceObject.Load(17, &i.queue)
	stateSourceObject.Load(18, &i.virtualOwner)
	stateSourceObject.Load(19, &i.haveBuf)
	stateSourceObject.Load(20, &i.buf)
	stateSourceObject.AfterLoad(i.afterLoad)
}

func (f *filesystemType) StateTypeName() string {
	return "pkg/sentry/fsimpl/host.filesystemType"
}

func (f *filesystemType) StateFields() []string {
	return []string{}
}

func (f *filesystemType) beforeSave() {}

// +checklocksignore
func (f *filesystemType) StateSave(stateSinkObject state.Sink) {
	f.beforeSave()
}

func (f *filesystemType) afterLoad() {}

// +checklocksignore
func (f *filesystemType) StateLoad(stateSourceObject state.Source) {
}

func (fs *filesystem) StateTypeName() string {
	return "pkg/sentry/fsimpl/host.filesystem"
}

func (fs *filesystem) StateFields() []string {
	return []string{
		"Filesystem",
		"devMinor",
	}
}

func (fs *filesystem) beforeSave() {}

// +checklocksignore
func (fs *filesystem) StateSave(stateSinkObject state.Sink) {
	fs.beforeSave()
	stateSinkObject.Save(0, &fs.Filesystem)
	stateSinkObject.Save(1, &fs.devMinor)
}

func (fs *filesystem) afterLoad() {}

// +checklocksignore
func (fs *filesystem) StateLoad(stateSourceObject state.Source) {
	stateSourceObject.Load(0, &fs.Filesystem)
	stateSourceObject.Load(1, &fs.devMinor)
}

func (f *fileDescription) StateTypeName() string {
	return "pkg/sentry/fsimpl/host.fileDescription"
}

func (f *fileDescription) StateFields() []string {
	return []string{
		"vfsfd",
		"FileDescriptionDefaultImpl",
		"LockFD",
		"inode",
		"offset",
	}
}

func (f *fileDescription) beforeSave() {}

// +checklocksignore
func (f *fileDescription) StateSave(stateSinkObject state.Sink) {
	f.beforeSave()
	stateSinkObject.Save(0, &f.vfsfd)
	stateSinkObject.Save(1, &f.FileDescriptionDefaultImpl)
	stateSinkObject.Save(2, &f.LockFD)
	stateSinkObject.Save(3, &f.inode)
	stateSinkObject.Save(4, &f.offset)
}

func (f *fileDescription) afterLoad() {}

// +checklocksignore
func (f *fileDescription) StateLoad(stateSourceObject state.Source) {
	stateSourceObject.Load(0, &f.vfsfd)
	stateSourceObject.Load(1, &f.FileDescriptionDefaultImpl)
	stateSourceObject.Load(2, &f.LockFD)
	stateSourceObject.Load(3, &f.inode)
	stateSourceObject.Load(4, &f.offset)
}

func (r *inodeRefs) StateTypeName() string {
	return "pkg/sentry/fsimpl/host.inodeRefs"
}

func (r *inodeRefs) StateFields() []string {
	return []string{
		"refCount",
	}
}

func (r *inodeRefs) beforeSave() {}

// +checklocksignore
func (r *inodeRefs) StateSave(stateSinkObject state.Sink) {
	r.beforeSave()
	stateSinkObject.Save(0, &r.refCount)
}

// +checklocksignore
func (r *inodeRefs) StateLoad(stateSourceObject state.Source) {
	stateSourceObject.Load(0, &r.refCount)
	stateSourceObject.AfterLoad(r.afterLoad)
}

func (t *TTYFileDescription) StateTypeName() string {
	return "pkg/sentry/fsimpl/host.TTYFileDescription"
}

func (t *TTYFileDescription) StateFields() []string {
	return []string{
		"fileDescription",
		"session",
		"fgProcessGroup",
		"termios",
	}
}

func (t *TTYFileDescription) beforeSave() {}

// +checklocksignore
func (t *TTYFileDescription) StateSave(stateSinkObject state.Sink) {
	t.beforeSave()
	stateSinkObject.Save(0, &t.fileDescription)
	stateSinkObject.Save(1, &t.session)
	stateSinkObject.Save(2, &t.fgProcessGroup)
	stateSinkObject.Save(3, &t.termios)
}

func (t *TTYFileDescription) afterLoad() {}

// +checklocksignore
func (t *TTYFileDescription) StateLoad(stateSourceObject state.Source) {
	stateSourceObject.Load(0, &t.fileDescription)
	stateSourceObject.Load(1, &t.session)
	stateSourceObject.Load(2, &t.fgProcessGroup)
	stateSourceObject.Load(3, &t.termios)
}

func init() {
	state.Register((*virtualOwner)(nil))
	state.Register((*inode)(nil))
	state.Register((*filesystemType)(nil))
	state.Register((*filesystem)(nil))
	state.Register((*fileDescription)(nil))
	state.Register((*inodeRefs)(nil))
	state.Register((*TTYFileDescription)(nil))
}
