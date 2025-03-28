package kernfs

import (
	"reflect"

	"github.com/talismancer/gvisor-ligolo/pkg/sync"
	"github.com/talismancer/gvisor-ligolo/pkg/sync/locking"
)

// Mutex is sync.Mutex with the correctness validator.
type deferredDecRefsMutex struct {
	mu sync.Mutex
}

var deferredDecRefsprefixIndex *locking.MutexClass

// lockNames is a list of user-friendly lock names.
// Populated in init.
var deferredDecRefslockNames []string

// lockNameIndex is used as an index passed to NestedLock and NestedUnlock,
// refering to an index within lockNames.
// Values are specified using the "consts" field of go_template_instance.
type deferredDecRefslockNameIndex int

// DO NOT REMOVE: The following function automatically replaced with lock index constants.
// LOCK_NAME_INDEX_CONSTANTS
const ()

// Lock locks m.
// +checklocksignore
func (m *deferredDecRefsMutex) Lock() {
	locking.AddGLock(deferredDecRefsprefixIndex, -1)
	m.mu.Lock()
}

// NestedLock locks m knowing that another lock of the same type is held.
// +checklocksignore
func (m *deferredDecRefsMutex) NestedLock(i deferredDecRefslockNameIndex) {
	locking.AddGLock(deferredDecRefsprefixIndex, int(i))
	m.mu.Lock()
}

// Unlock unlocks m.
// +checklocksignore
func (m *deferredDecRefsMutex) Unlock() {
	locking.DelGLock(deferredDecRefsprefixIndex, -1)
	m.mu.Unlock()
}

// NestedUnlock unlocks m knowing that another lock of the same type is held.
// +checklocksignore
func (m *deferredDecRefsMutex) NestedUnlock(i deferredDecRefslockNameIndex) {
	locking.DelGLock(deferredDecRefsprefixIndex, int(i))
	m.mu.Unlock()
}

// DO NOT REMOVE: The following function is automatically replaced.
func deferredDecRefsinitLockNames() {}

func init() {
	deferredDecRefsinitLockNames()
	deferredDecRefsprefixIndex = locking.NewMutexClass(reflect.TypeOf(deferredDecRefsMutex{}), deferredDecRefslockNames)
}
