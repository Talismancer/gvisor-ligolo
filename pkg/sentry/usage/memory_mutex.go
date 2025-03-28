package usage

import (
	"reflect"

	"github.com/talismancer/gvisor-ligolo/pkg/sync"
	"github.com/talismancer/gvisor-ligolo/pkg/sync/locking"
)

// Mutex is sync.Mutex with the correctness validator.
type memoryMutex struct {
	mu sync.Mutex
}

var memoryprefixIndex *locking.MutexClass

// lockNames is a list of user-friendly lock names.
// Populated in init.
var memorylockNames []string

// lockNameIndex is used as an index passed to NestedLock and NestedUnlock,
// refering to an index within lockNames.
// Values are specified using the "consts" field of go_template_instance.
type memorylockNameIndex int

// DO NOT REMOVE: The following function automatically replaced with lock index constants.
// LOCK_NAME_INDEX_CONSTANTS
const ()

// Lock locks m.
// +checklocksignore
func (m *memoryMutex) Lock() {
	locking.AddGLock(memoryprefixIndex, -1)
	m.mu.Lock()
}

// NestedLock locks m knowing that another lock of the same type is held.
// +checklocksignore
func (m *memoryMutex) NestedLock(i memorylockNameIndex) {
	locking.AddGLock(memoryprefixIndex, int(i))
	m.mu.Lock()
}

// Unlock unlocks m.
// +checklocksignore
func (m *memoryMutex) Unlock() {
	locking.DelGLock(memoryprefixIndex, -1)
	m.mu.Unlock()
}

// NestedUnlock unlocks m knowing that another lock of the same type is held.
// +checklocksignore
func (m *memoryMutex) NestedUnlock(i memorylockNameIndex) {
	locking.DelGLock(memoryprefixIndex, int(i))
	m.mu.Unlock()
}

// DO NOT REMOVE: The following function is automatically replaced.
func memoryinitLockNames() {}

func init() {
	memoryinitLockNames()
	memoryprefixIndex = locking.NewMutexClass(reflect.TypeOf(memoryMutex{}), memorylockNames)
}
