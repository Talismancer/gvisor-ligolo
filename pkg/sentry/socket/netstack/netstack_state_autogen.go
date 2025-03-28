// automatically generated by stateify.

package netstack

import (
	"github.com/talismancer/gvisor-ligolo/pkg/state"
)

func (s *sock) StateTypeName() string {
	return "pkg/sentry/socket/netstack.sock"
}

func (s *sock) StateFields() []string {
	return []string{
		"vfsfd",
		"FileDescriptionDefaultImpl",
		"DentryMetadataFileDescriptionImpl",
		"LockFD",
		"SendReceiveTimeout",
		"Queue",
		"family",
		"Endpoint",
		"skType",
		"protocol",
		"namespace",
		"sockOptTimestamp",
		"timestampValid",
		"timestamp",
		"sockOptInq",
	}
}

func (s *sock) beforeSave() {}

// +checklocksignore
func (s *sock) StateSave(stateSinkObject state.Sink) {
	s.beforeSave()
	var timestampValue int64
	timestampValue = s.saveTimestamp()
	stateSinkObject.SaveValue(13, timestampValue)
	stateSinkObject.Save(0, &s.vfsfd)
	stateSinkObject.Save(1, &s.FileDescriptionDefaultImpl)
	stateSinkObject.Save(2, &s.DentryMetadataFileDescriptionImpl)
	stateSinkObject.Save(3, &s.LockFD)
	stateSinkObject.Save(4, &s.SendReceiveTimeout)
	stateSinkObject.Save(5, &s.Queue)
	stateSinkObject.Save(6, &s.family)
	stateSinkObject.Save(7, &s.Endpoint)
	stateSinkObject.Save(8, &s.skType)
	stateSinkObject.Save(9, &s.protocol)
	stateSinkObject.Save(10, &s.namespace)
	stateSinkObject.Save(11, &s.sockOptTimestamp)
	stateSinkObject.Save(12, &s.timestampValid)
	stateSinkObject.Save(14, &s.sockOptInq)
}

func (s *sock) afterLoad() {}

// +checklocksignore
func (s *sock) StateLoad(stateSourceObject state.Source) {
	stateSourceObject.Load(0, &s.vfsfd)
	stateSourceObject.Load(1, &s.FileDescriptionDefaultImpl)
	stateSourceObject.Load(2, &s.DentryMetadataFileDescriptionImpl)
	stateSourceObject.Load(3, &s.LockFD)
	stateSourceObject.Load(4, &s.SendReceiveTimeout)
	stateSourceObject.Load(5, &s.Queue)
	stateSourceObject.Load(6, &s.family)
	stateSourceObject.Load(7, &s.Endpoint)
	stateSourceObject.Load(8, &s.skType)
	stateSourceObject.Load(9, &s.protocol)
	stateSourceObject.Load(10, &s.namespace)
	stateSourceObject.Load(11, &s.sockOptTimestamp)
	stateSourceObject.Load(12, &s.timestampValid)
	stateSourceObject.Load(14, &s.sockOptInq)
	stateSourceObject.LoadValue(13, new(int64), func(y any) { s.loadTimestamp(y.(int64)) })
}

func (s *Stack) StateTypeName() string {
	return "pkg/sentry/socket/netstack.Stack"
}

func (s *Stack) StateFields() []string {
	return []string{}
}

func (s *Stack) beforeSave() {}

// +checklocksignore
func (s *Stack) StateSave(stateSinkObject state.Sink) {
	s.beforeSave()
}

// +checklocksignore
func (s *Stack) StateLoad(stateSourceObject state.Source) {
	stateSourceObject.AfterLoad(s.afterLoad)
}

func init() {
	state.Register((*sock)(nil))
	state.Register((*Stack)(nil))
}
