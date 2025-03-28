// automatically generated by stateify.

package fifo

import (
	"github.com/talismancer/gvisor-ligolo/pkg/state"
)

func (pl *packetBufferCircularList) StateTypeName() string {
	return "pkg/tcpip/link/qdisc/fifo.packetBufferCircularList"
}

func (pl *packetBufferCircularList) StateFields() []string {
	return []string{
		"pbs",
		"head",
		"size",
	}
}

func (pl *packetBufferCircularList) beforeSave() {}

// +checklocksignore
func (pl *packetBufferCircularList) StateSave(stateSinkObject state.Sink) {
	pl.beforeSave()
	stateSinkObject.Save(0, &pl.pbs)
	stateSinkObject.Save(1, &pl.head)
	stateSinkObject.Save(2, &pl.size)
}

func (pl *packetBufferCircularList) afterLoad() {}

// +checklocksignore
func (pl *packetBufferCircularList) StateLoad(stateSourceObject state.Source) {
	stateSourceObject.Load(0, &pl.pbs)
	stateSourceObject.Load(1, &pl.head)
	stateSourceObject.Load(2, &pl.size)
}

func init() {
	state.Register((*packetBufferCircularList)(nil))
}
