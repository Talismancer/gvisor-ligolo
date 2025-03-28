// automatically generated by stateify.

package port

import (
	"github.com/talismancer/gvisor-ligolo/pkg/state"
)

func (m *Manager) StateTypeName() string {
	return "pkg/sentry/socket/netlink/port.Manager"
}

func (m *Manager) StateFields() []string {
	return []string{
		"ports",
	}
}

func (m *Manager) beforeSave() {}

// +checklocksignore
func (m *Manager) StateSave(stateSinkObject state.Sink) {
	m.beforeSave()
	stateSinkObject.Save(0, &m.ports)
}

func (m *Manager) afterLoad() {}

// +checklocksignore
func (m *Manager) StateLoad(stateSourceObject state.Source) {
	stateSourceObject.Load(0, &m.ports)
}

func init() {
	state.Register((*Manager)(nil))
}
