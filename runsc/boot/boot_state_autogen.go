// automatically generated by stateify.

package boot

import (
	"github.com/talismancer/gvisor-ligolo/pkg/state"
)

func (f *sandboxNetstackCreator) StateTypeName() string {
	return "runsc/boot.sandboxNetstackCreator"
}

func (f *sandboxNetstackCreator) StateFields() []string {
	return []string{
		"clock",
		"uniqueID",
		"allowPacketEndpointWrite",
	}
}

func (f *sandboxNetstackCreator) beforeSave() {}

// +checklocksignore
func (f *sandboxNetstackCreator) StateSave(stateSinkObject state.Sink) {
	f.beforeSave()
	stateSinkObject.Save(0, &f.clock)
	stateSinkObject.Save(1, &f.uniqueID)
	stateSinkObject.Save(2, &f.allowPacketEndpointWrite)
}

func (f *sandboxNetstackCreator) afterLoad() {}

// +checklocksignore
func (f *sandboxNetstackCreator) StateLoad(stateSourceObject state.Source) {
	stateSourceObject.Load(0, &f.clock)
	stateSourceObject.Load(1, &f.uniqueID)
	stateSourceObject.Load(2, &f.allowPacketEndpointWrite)
}

func init() {
	state.Register((*sandboxNetstackCreator)(nil))
}
