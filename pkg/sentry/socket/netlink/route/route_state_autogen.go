// automatically generated by stateify.

package route

import (
	"github.com/talismancer/gvisor-ligolo/pkg/state"
)

func (p *Protocol) StateTypeName() string {
	return "pkg/sentry/socket/netlink/route.Protocol"
}

func (p *Protocol) StateFields() []string {
	return []string{}
}

func (p *Protocol) beforeSave() {}

// +checklocksignore
func (p *Protocol) StateSave(stateSinkObject state.Sink) {
	p.beforeSave()
}

func (p *Protocol) afterLoad() {}

// +checklocksignore
func (p *Protocol) StateLoad(stateSourceObject state.Source) {
}

func init() {
	state.Register((*Protocol)(nil))
}
