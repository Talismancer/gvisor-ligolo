package pgalloc

import (
	"bytes"
	"fmt"

	__generics_imported0 "github.com/talismancer/gvisor-ligolo/pkg/sentry/memmap"
)

// trackGaps is an optional parameter.
//
// If trackGaps is 1, the Set will track maximum gap size recursively,
// enabling the GapIterator.{Prev,Next}LargeEnoughGap functions. In this
// case, Key must be an unsigned integer.
//
// trackGaps must be 0 or 1.
const reclaimtrackGaps = 0

var _ = uint8(reclaimtrackGaps << 7) // Will fail if not zero or one.

// dynamicGap is a type that disappears if trackGaps is 0.
type reclaimdynamicGap [reclaimtrackGaps]uint64

// Get returns the value of the gap.
//
// Precondition: trackGaps must be non-zero.
func (d *reclaimdynamicGap) Get() uint64 {
	return d[:][0]
}

// Set sets the value of the gap.
//
// Precondition: trackGaps must be non-zero.
func (d *reclaimdynamicGap) Set(v uint64) {
	d[:][0] = v
}

const (
	// minDegree is the minimum degree of an internal node in a Set B-tree.
	//
	//	- Any non-root node has at least minDegree-1 segments.
	//
	//	- Any non-root internal (non-leaf) node has at least minDegree children.
	//
	//	- The root node may have fewer than minDegree-1 segments, but it may
	// only have 0 segments if the tree is empty.
	//
	// Our implementation requires minDegree >= 3. Higher values of minDegree
	// usually improve performance, but increase memory usage for small sets.
	reclaimminDegree = 10

	reclaimmaxDegree = 2 * reclaimminDegree
)

// A Set is a mapping of segments with non-overlapping Range keys. The zero
// value for a Set is an empty set. Set values are not safely movable nor
// copyable. Set is thread-compatible.
//
// +stateify savable
type reclaimSet struct {
	root reclaimnode `state:".(*reclaimSegmentDataSlices)"`
}

// IsEmpty returns true if the set contains no segments.
func (s *reclaimSet) IsEmpty() bool {
	return s.root.nrSegments == 0
}

// IsEmptyRange returns true iff no segments in the set overlap the given
// range. This is semantically equivalent to s.SpanRange(r) == 0, but may be
// more efficient.
func (s *reclaimSet) IsEmptyRange(r __generics_imported0.FileRange) bool {
	switch {
	case r.Length() < 0:
		panic(fmt.Sprintf("invalid range %v", r))
	case r.Length() == 0:
		return true
	}
	_, gap := s.Find(r.Start)
	if !gap.Ok() {
		return false
	}
	return r.End <= gap.End()
}

// Span returns the total size of all segments in the set.
func (s *reclaimSet) Span() uint64 {
	var sz uint64
	for seg := s.FirstSegment(); seg.Ok(); seg = seg.NextSegment() {
		sz += seg.Range().Length()
	}
	return sz
}

// SpanRange returns the total size of the intersection of segments in the set
// with the given range.
func (s *reclaimSet) SpanRange(r __generics_imported0.FileRange) uint64 {
	switch {
	case r.Length() < 0:
		panic(fmt.Sprintf("invalid range %v", r))
	case r.Length() == 0:
		return 0
	}
	var sz uint64
	for seg := s.LowerBoundSegment(r.Start); seg.Ok() && seg.Start() < r.End; seg = seg.NextSegment() {
		sz += seg.Range().Intersect(r).Length()
	}
	return sz
}

// FirstSegment returns the first segment in the set. If the set is empty,
// FirstSegment returns a terminal iterator.
func (s *reclaimSet) FirstSegment() reclaimIterator {
	if s.root.nrSegments == 0 {
		return reclaimIterator{}
	}
	return s.root.firstSegment()
}

// LastSegment returns the last segment in the set. If the set is empty,
// LastSegment returns a terminal iterator.
func (s *reclaimSet) LastSegment() reclaimIterator {
	if s.root.nrSegments == 0 {
		return reclaimIterator{}
	}
	return s.root.lastSegment()
}

// FirstGap returns the first gap in the set.
func (s *reclaimSet) FirstGap() reclaimGapIterator {
	n := &s.root
	for n.hasChildren {
		n = n.children[0]
	}
	return reclaimGapIterator{n, 0}
}

// LastGap returns the last gap in the set.
func (s *reclaimSet) LastGap() reclaimGapIterator {
	n := &s.root
	for n.hasChildren {
		n = n.children[n.nrSegments]
	}
	return reclaimGapIterator{n, n.nrSegments}
}

// Find returns the segment or gap whose range contains the given key. If a
// segment is found, the returned Iterator is non-terminal and the
// returned GapIterator is terminal. Otherwise, the returned Iterator is
// terminal and the returned GapIterator is non-terminal.
func (s *reclaimSet) Find(key uint64) (reclaimIterator, reclaimGapIterator) {
	n := &s.root
	for {

		lower := 0
		upper := n.nrSegments
		for lower < upper {
			i := lower + (upper-lower)/2
			if r := n.keys[i]; key < r.End {
				if key >= r.Start {
					return reclaimIterator{n, i}, reclaimGapIterator{}
				}
				upper = i
			} else {
				lower = i + 1
			}
		}
		i := lower
		if !n.hasChildren {
			return reclaimIterator{}, reclaimGapIterator{n, i}
		}
		n = n.children[i]
	}
}

// FindSegment returns the segment whose range contains the given key. If no
// such segment exists, FindSegment returns a terminal iterator.
func (s *reclaimSet) FindSegment(key uint64) reclaimIterator {
	seg, _ := s.Find(key)
	return seg
}

// LowerBoundSegment returns the segment with the lowest range that contains a
// key greater than or equal to min. If no such segment exists,
// LowerBoundSegment returns a terminal iterator.
func (s *reclaimSet) LowerBoundSegment(min uint64) reclaimIterator {
	seg, gap := s.Find(min)
	if seg.Ok() {
		return seg
	}
	return gap.NextSegment()
}

// UpperBoundSegment returns the segment with the highest range that contains a
// key less than or equal to max. If no such segment exists, UpperBoundSegment
// returns a terminal iterator.
func (s *reclaimSet) UpperBoundSegment(max uint64) reclaimIterator {
	seg, gap := s.Find(max)
	if seg.Ok() {
		return seg
	}
	return gap.PrevSegment()
}

// FindGap returns the gap containing the given key. If no such gap exists
// (i.e. the set contains a segment containing that key), FindGap returns a
// terminal iterator.
func (s *reclaimSet) FindGap(key uint64) reclaimGapIterator {
	_, gap := s.Find(key)
	return gap
}

// LowerBoundGap returns the gap with the lowest range that is greater than or
// equal to min.
func (s *reclaimSet) LowerBoundGap(min uint64) reclaimGapIterator {
	seg, gap := s.Find(min)
	if gap.Ok() {
		return gap
	}
	return seg.NextGap()
}

// UpperBoundGap returns the gap with the highest range that is less than or
// equal to max.
func (s *reclaimSet) UpperBoundGap(max uint64) reclaimGapIterator {
	seg, gap := s.Find(max)
	if gap.Ok() {
		return gap
	}
	return seg.PrevGap()
}

// Add inserts the given segment into the set and returns true. If the new
// segment can be merged with adjacent segments, Add will do so. If the new
// segment would overlap an existing segment, Add returns false. If Add
// succeeds, all existing iterators are invalidated.
func (s *reclaimSet) Add(r __generics_imported0.FileRange, val reclaimSetValue) bool {
	if r.Length() <= 0 {
		panic(fmt.Sprintf("invalid segment range %v", r))
	}
	gap := s.FindGap(r.Start)
	if !gap.Ok() {
		return false
	}
	if r.End > gap.End() {
		return false
	}
	s.Insert(gap, r, val)
	return true
}

// AddWithoutMerging inserts the given segment into the set and returns true.
// If it would overlap an existing segment, AddWithoutMerging does nothing and
// returns false. If AddWithoutMerging succeeds, all existing iterators are
// invalidated.
func (s *reclaimSet) AddWithoutMerging(r __generics_imported0.FileRange, val reclaimSetValue) bool {
	if r.Length() <= 0 {
		panic(fmt.Sprintf("invalid segment range %v", r))
	}
	gap := s.FindGap(r.Start)
	if !gap.Ok() {
		return false
	}
	if r.End > gap.End() {
		return false
	}
	s.InsertWithoutMergingUnchecked(gap, r, val)
	return true
}

// Insert inserts the given segment into the given gap. If the new segment can
// be merged with adjacent segments, Insert will do so. Insert returns an
// iterator to the segment containing the inserted value (which may have been
// merged with other values). All existing iterators (including gap, but not
// including the returned iterator) are invalidated.
//
// If the gap cannot accommodate the segment, or if r is invalid, Insert panics.
//
// Insert is semantically equivalent to a InsertWithoutMerging followed by a
// Merge, but may be more efficient. Note that there is no unchecked variant of
// Insert since Insert must retrieve and inspect gap's predecessor and
// successor segments regardless.
func (s *reclaimSet) Insert(gap reclaimGapIterator, r __generics_imported0.FileRange, val reclaimSetValue) reclaimIterator {
	if r.Length() <= 0 {
		panic(fmt.Sprintf("invalid segment range %v", r))
	}
	prev, next := gap.PrevSegment(), gap.NextSegment()
	if prev.Ok() && prev.End() > r.Start {
		panic(fmt.Sprintf("new segment %v overlaps predecessor %v", r, prev.Range()))
	}
	if next.Ok() && next.Start() < r.End {
		panic(fmt.Sprintf("new segment %v overlaps successor %v", r, next.Range()))
	}
	if prev.Ok() && prev.End() == r.Start {
		if mval, ok := (reclaimSetFunctions{}).Merge(prev.Range(), prev.Value(), r, val); ok {
			shrinkMaxGap := reclaimtrackGaps != 0 && gap.Range().Length() == gap.node.maxGap.Get()
			prev.SetEndUnchecked(r.End)
			prev.SetValue(mval)
			if shrinkMaxGap {
				gap.node.updateMaxGapLeaf()
			}
			if next.Ok() && next.Start() == r.End {
				val = mval
				if mval, ok := (reclaimSetFunctions{}).Merge(prev.Range(), val, next.Range(), next.Value()); ok {
					prev.SetEndUnchecked(next.End())
					prev.SetValue(mval)
					return s.Remove(next).PrevSegment()
				}
			}
			return prev
		}
	}
	if next.Ok() && next.Start() == r.End {
		if mval, ok := (reclaimSetFunctions{}).Merge(r, val, next.Range(), next.Value()); ok {
			shrinkMaxGap := reclaimtrackGaps != 0 && gap.Range().Length() == gap.node.maxGap.Get()
			next.SetStartUnchecked(r.Start)
			next.SetValue(mval)
			if shrinkMaxGap {
				gap.node.updateMaxGapLeaf()
			}
			return next
		}
	}

	return s.InsertWithoutMergingUnchecked(gap, r, val)
}

// InsertWithoutMerging inserts the given segment into the given gap and
// returns an iterator to the inserted segment. All existing iterators
// (including gap, but not including the returned iterator) are invalidated.
//
// If the gap cannot accommodate the segment, or if r is invalid,
// InsertWithoutMerging panics.
func (s *reclaimSet) InsertWithoutMerging(gap reclaimGapIterator, r __generics_imported0.FileRange, val reclaimSetValue) reclaimIterator {
	if r.Length() <= 0 {
		panic(fmt.Sprintf("invalid segment range %v", r))
	}
	if gr := gap.Range(); !gr.IsSupersetOf(r) {
		panic(fmt.Sprintf("cannot insert segment range %v into gap range %v", r, gr))
	}
	return s.InsertWithoutMergingUnchecked(gap, r, val)
}

// InsertWithoutMergingUnchecked inserts the given segment into the given gap
// and returns an iterator to the inserted segment. All existing iterators
// (including gap, but not including the returned iterator) are invalidated.
//
// Preconditions:
//   - r.Start >= gap.Start().
//   - r.End <= gap.End().
func (s *reclaimSet) InsertWithoutMergingUnchecked(gap reclaimGapIterator, r __generics_imported0.FileRange, val reclaimSetValue) reclaimIterator {
	gap = gap.node.rebalanceBeforeInsert(gap)
	splitMaxGap := reclaimtrackGaps != 0 && (gap.node.nrSegments == 0 || gap.Range().Length() == gap.node.maxGap.Get())
	copy(gap.node.keys[gap.index+1:], gap.node.keys[gap.index:gap.node.nrSegments])
	copy(gap.node.values[gap.index+1:], gap.node.values[gap.index:gap.node.nrSegments])
	gap.node.keys[gap.index] = r
	gap.node.values[gap.index] = val
	gap.node.nrSegments++
	if splitMaxGap {
		gap.node.updateMaxGapLeaf()
	}
	return reclaimIterator{gap.node, gap.index}
}

// Remove removes the given segment and returns an iterator to the vacated gap.
// All existing iterators (including seg, but not including the returned
// iterator) are invalidated.
func (s *reclaimSet) Remove(seg reclaimIterator) reclaimGapIterator {

	if seg.node.hasChildren {

		victim := seg.PrevSegment()

		seg.SetRangeUnchecked(victim.Range())
		seg.SetValue(victim.Value())

		nextAdjacentNode := seg.NextSegment().node
		if reclaimtrackGaps != 0 {
			nextAdjacentNode.updateMaxGapLeaf()
		}
		return s.Remove(victim).NextGap()
	}
	copy(seg.node.keys[seg.index:], seg.node.keys[seg.index+1:seg.node.nrSegments])
	copy(seg.node.values[seg.index:], seg.node.values[seg.index+1:seg.node.nrSegments])
	reclaimSetFunctions{}.ClearValue(&seg.node.values[seg.node.nrSegments-1])
	seg.node.nrSegments--
	if reclaimtrackGaps != 0 {
		seg.node.updateMaxGapLeaf()
	}
	return seg.node.rebalanceAfterRemove(reclaimGapIterator{seg.node, seg.index})
}

// RemoveAll removes all segments from the set. All existing iterators are
// invalidated.
func (s *reclaimSet) RemoveAll() {
	s.root = reclaimnode{}
}

// RemoveRange removes all segments in the given range. An iterator to the
// newly formed gap is returned, and all existing iterators are invalidated.
func (s *reclaimSet) RemoveRange(r __generics_imported0.FileRange) reclaimGapIterator {
	seg, gap := s.Find(r.Start)
	if seg.Ok() {
		seg = s.Isolate(seg, r)
		gap = s.Remove(seg)
	}
	for seg = gap.NextSegment(); seg.Ok() && seg.Start() < r.End; seg = gap.NextSegment() {
		seg = s.Isolate(seg, r)
		gap = s.Remove(seg)
	}
	return gap
}

// Merge attempts to merge two neighboring segments. If successful, Merge
// returns an iterator to the merged segment, and all existing iterators are
// invalidated. Otherwise, Merge returns a terminal iterator.
//
// If first is not the predecessor of second, Merge panics.
func (s *reclaimSet) Merge(first, second reclaimIterator) reclaimIterator {
	if first.NextSegment() != second {
		panic(fmt.Sprintf("attempt to merge non-neighboring segments %v, %v", first.Range(), second.Range()))
	}
	return s.MergeUnchecked(first, second)
}

// MergeUnchecked attempts to merge two neighboring segments. If successful,
// MergeUnchecked returns an iterator to the merged segment, and all existing
// iterators are invalidated. Otherwise, MergeUnchecked returns a terminal
// iterator.
//
// Precondition: first is the predecessor of second: first.NextSegment() ==
// second, first == second.PrevSegment().
func (s *reclaimSet) MergeUnchecked(first, second reclaimIterator) reclaimIterator {
	if first.End() == second.Start() {
		if mval, ok := (reclaimSetFunctions{}).Merge(first.Range(), first.Value(), second.Range(), second.Value()); ok {

			first.SetEndUnchecked(second.End())
			first.SetValue(mval)

			return s.Remove(second).PrevSegment()
		}
	}
	return reclaimIterator{}
}

// MergeAll attempts to merge all adjacent segments in the set. All existing
// iterators are invalidated.
func (s *reclaimSet) MergeAll() {
	seg := s.FirstSegment()
	if !seg.Ok() {
		return
	}
	next := seg.NextSegment()
	for next.Ok() {
		if mseg := s.MergeUnchecked(seg, next); mseg.Ok() {
			seg, next = mseg, mseg.NextSegment()
		} else {
			seg, next = next, next.NextSegment()
		}
	}
}

// MergeRange attempts to merge all adjacent segments that contain a key in the
// specific range. All existing iterators are invalidated.
func (s *reclaimSet) MergeRange(r __generics_imported0.FileRange) {
	seg := s.LowerBoundSegment(r.Start)
	if !seg.Ok() {
		return
	}
	next := seg.NextSegment()
	for next.Ok() && next.Range().Start < r.End {
		if mseg := s.MergeUnchecked(seg, next); mseg.Ok() {
			seg, next = mseg, mseg.NextSegment()
		} else {
			seg, next = next, next.NextSegment()
		}
	}
}

// MergeAdjacent attempts to merge the segment containing r.Start with its
// predecessor, and the segment containing r.End-1 with its successor.
func (s *reclaimSet) MergeAdjacent(r __generics_imported0.FileRange) {
	first := s.FindSegment(r.Start)
	if first.Ok() {
		if prev := first.PrevSegment(); prev.Ok() {
			s.Merge(prev, first)
		}
	}
	last := s.FindSegment(r.End - 1)
	if last.Ok() {
		if next := last.NextSegment(); next.Ok() {
			s.Merge(last, next)
		}
	}
}

// Split splits the given segment at the given key and returns iterators to the
// two resulting segments. All existing iterators (including seg, but not
// including the returned iterators) are invalidated.
//
// If the segment cannot be split at split (because split is at the start or
// end of the segment's range, so splitting would produce a segment with zero
// length, or because split falls outside the segment's range altogether),
// Split panics.
func (s *reclaimSet) Split(seg reclaimIterator, split uint64) (reclaimIterator, reclaimIterator) {
	if !seg.Range().CanSplitAt(split) {
		panic(fmt.Sprintf("can't split %v at %v", seg.Range(), split))
	}
	return s.SplitUnchecked(seg, split)
}

// SplitUnchecked splits the given segment at the given key and returns
// iterators to the two resulting segments. All existing iterators (including
// seg, but not including the returned iterators) are invalidated.
//
// Preconditions: seg.Start() < key < seg.End().
func (s *reclaimSet) SplitUnchecked(seg reclaimIterator, split uint64) (reclaimIterator, reclaimIterator) {
	val1, val2 := (reclaimSetFunctions{}).Split(seg.Range(), seg.Value(), split)
	end2 := seg.End()
	seg.SetEndUnchecked(split)
	seg.SetValue(val1)
	seg2 := s.InsertWithoutMergingUnchecked(seg.NextGap(), __generics_imported0.FileRange{split, end2}, val2)

	return seg2.PrevSegment(), seg2
}

// SplitAt splits the segment straddling split, if one exists. SplitAt returns
// true if a segment was split and false otherwise. If SplitAt splits a
// segment, all existing iterators are invalidated.
func (s *reclaimSet) SplitAt(split uint64) bool {
	if seg := s.FindSegment(split); seg.Ok() && seg.Range().CanSplitAt(split) {
		s.SplitUnchecked(seg, split)
		return true
	}
	return false
}

// Isolate ensures that the given segment's range does not escape r by
// splitting at r.Start and r.End if necessary, and returns an updated iterator
// to the bounded segment. All existing iterators (including seg, but not
// including the returned iterators) are invalidated.
func (s *reclaimSet) Isolate(seg reclaimIterator, r __generics_imported0.FileRange) reclaimIterator {
	if seg.Range().CanSplitAt(r.Start) {
		_, seg = s.SplitUnchecked(seg, r.Start)
	}
	if seg.Range().CanSplitAt(r.End) {
		seg, _ = s.SplitUnchecked(seg, r.End)
	}
	return seg
}

// ApplyContiguous applies a function to a contiguous range of segments,
// splitting if necessary. The function is applied until the first gap is
// encountered, at which point the gap is returned. If the function is applied
// across the entire range, a terminal gap is returned. All existing iterators
// are invalidated.
//
// N.B. The Iterator must not be invalidated by the function.
func (s *reclaimSet) ApplyContiguous(r __generics_imported0.FileRange, fn func(seg reclaimIterator)) reclaimGapIterator {
	seg, gap := s.Find(r.Start)
	if !seg.Ok() {
		return gap
	}
	for {
		seg = s.Isolate(seg, r)
		fn(seg)
		if seg.End() >= r.End {
			return reclaimGapIterator{}
		}
		gap = seg.NextGap()
		if !gap.IsEmpty() {
			return gap
		}
		seg = gap.NextSegment()
		if !seg.Ok() {

			return reclaimGapIterator{}
		}
	}
}

// +stateify savable
type reclaimnode struct {
	// An internal binary tree node looks like:
	//
	//   K
	//  / \
	// Cl Cr
	//
	// where all keys in the subtree rooted by Cl (the left subtree) are less
	// than K (the key of the parent node), and all keys in the subtree rooted
	// by Cr (the right subtree) are greater than K.
	//
	// An internal B-tree node's indexes work out to look like:
	//
	//   K0 K1 K2  ...   Kn-1
	//  / \/ \/ \  ...  /  \
	// C0 C1 C2 C3 ... Cn-1 Cn
	//
	// where n is nrSegments.
	nrSegments int

	// parent is a pointer to this node's parent. If this node is root, parent
	// is nil.
	parent *reclaimnode

	// parentIndex is the index of this node in parent.children.
	parentIndex int

	// Flag for internal nodes that is technically redundant with "children[0]
	// != nil", but is stored in the first cache line. "hasChildren" rather
	// than "isLeaf" because false must be the correct value for an empty root.
	hasChildren bool

	// The longest gap within this node. If the node is a leaf, it's simply the
	// maximum gap among all the (nrSegments+1) gaps formed by its nrSegments keys
	// including the 0th and nrSegments-th gap possibly shared with its upper-level
	// nodes; if it's a non-leaf node, it's the max of all children's maxGap.
	maxGap reclaimdynamicGap

	// Nodes store keys and values in separate arrays to maximize locality in
	// the common case (scanning keys for lookup).
	keys     [reclaimmaxDegree - 1]__generics_imported0.FileRange
	values   [reclaimmaxDegree - 1]reclaimSetValue
	children [reclaimmaxDegree]*reclaimnode
}

// firstSegment returns the first segment in the subtree rooted by n.
//
// Preconditions: n.nrSegments != 0.
func (n *reclaimnode) firstSegment() reclaimIterator {
	for n.hasChildren {
		n = n.children[0]
	}
	return reclaimIterator{n, 0}
}

// lastSegment returns the last segment in the subtree rooted by n.
//
// Preconditions: n.nrSegments != 0.
func (n *reclaimnode) lastSegment() reclaimIterator {
	for n.hasChildren {
		n = n.children[n.nrSegments]
	}
	return reclaimIterator{n, n.nrSegments - 1}
}

func (n *reclaimnode) prevSibling() *reclaimnode {
	if n.parent == nil || n.parentIndex == 0 {
		return nil
	}
	return n.parent.children[n.parentIndex-1]
}

func (n *reclaimnode) nextSibling() *reclaimnode {
	if n.parent == nil || n.parentIndex == n.parent.nrSegments {
		return nil
	}
	return n.parent.children[n.parentIndex+1]
}

// rebalanceBeforeInsert splits n and its ancestors if they are full, as
// required for insertion, and returns an updated iterator to the position
// represented by gap.
func (n *reclaimnode) rebalanceBeforeInsert(gap reclaimGapIterator) reclaimGapIterator {
	if n.nrSegments < reclaimmaxDegree-1 {
		return gap
	}
	if n.parent != nil {
		gap = n.parent.rebalanceBeforeInsert(gap)
	}
	if n.parent == nil {

		left := &reclaimnode{
			nrSegments:  reclaimminDegree - 1,
			parent:      n,
			parentIndex: 0,
			hasChildren: n.hasChildren,
		}
		right := &reclaimnode{
			nrSegments:  reclaimminDegree - 1,
			parent:      n,
			parentIndex: 1,
			hasChildren: n.hasChildren,
		}
		copy(left.keys[:reclaimminDegree-1], n.keys[:reclaimminDegree-1])
		copy(left.values[:reclaimminDegree-1], n.values[:reclaimminDegree-1])
		copy(right.keys[:reclaimminDegree-1], n.keys[reclaimminDegree:])
		copy(right.values[:reclaimminDegree-1], n.values[reclaimminDegree:])
		n.keys[0], n.values[0] = n.keys[reclaimminDegree-1], n.values[reclaimminDegree-1]
		reclaimzeroValueSlice(n.values[1:])
		if n.hasChildren {
			copy(left.children[:reclaimminDegree], n.children[:reclaimminDegree])
			copy(right.children[:reclaimminDegree], n.children[reclaimminDegree:])
			reclaimzeroNodeSlice(n.children[2:])
			for i := 0; i < reclaimminDegree; i++ {
				left.children[i].parent = left
				left.children[i].parentIndex = i
				right.children[i].parent = right
				right.children[i].parentIndex = i
			}
		}
		n.nrSegments = 1
		n.hasChildren = true
		n.children[0] = left
		n.children[1] = right

		if reclaimtrackGaps != 0 {
			left.updateMaxGapLocal()
			right.updateMaxGapLocal()
		}
		if gap.node != n {
			return gap
		}
		if gap.index < reclaimminDegree {
			return reclaimGapIterator{left, gap.index}
		}
		return reclaimGapIterator{right, gap.index - reclaimminDegree}
	}

	copy(n.parent.keys[n.parentIndex+1:], n.parent.keys[n.parentIndex:n.parent.nrSegments])
	copy(n.parent.values[n.parentIndex+1:], n.parent.values[n.parentIndex:n.parent.nrSegments])
	n.parent.keys[n.parentIndex], n.parent.values[n.parentIndex] = n.keys[reclaimminDegree-1], n.values[reclaimminDegree-1]
	copy(n.parent.children[n.parentIndex+2:], n.parent.children[n.parentIndex+1:n.parent.nrSegments+1])
	for i := n.parentIndex + 2; i < n.parent.nrSegments+2; i++ {
		n.parent.children[i].parentIndex = i
	}
	sibling := &reclaimnode{
		nrSegments:  reclaimminDegree - 1,
		parent:      n.parent,
		parentIndex: n.parentIndex + 1,
		hasChildren: n.hasChildren,
	}
	n.parent.children[n.parentIndex+1] = sibling
	n.parent.nrSegments++
	copy(sibling.keys[:reclaimminDegree-1], n.keys[reclaimminDegree:])
	copy(sibling.values[:reclaimminDegree-1], n.values[reclaimminDegree:])
	reclaimzeroValueSlice(n.values[reclaimminDegree-1:])
	if n.hasChildren {
		copy(sibling.children[:reclaimminDegree], n.children[reclaimminDegree:])
		reclaimzeroNodeSlice(n.children[reclaimminDegree:])
		for i := 0; i < reclaimminDegree; i++ {
			sibling.children[i].parent = sibling
			sibling.children[i].parentIndex = i
		}
	}
	n.nrSegments = reclaimminDegree - 1

	if reclaimtrackGaps != 0 {
		n.updateMaxGapLocal()
		sibling.updateMaxGapLocal()
	}

	if gap.node != n {
		return gap
	}
	if gap.index < reclaimminDegree {
		return gap
	}
	return reclaimGapIterator{sibling, gap.index - reclaimminDegree}
}

// rebalanceAfterRemove "unsplits" n and its ancestors if they are deficient
// (contain fewer segments than required by B-tree invariants), as required for
// removal, and returns an updated iterator to the position represented by gap.
//
// Precondition: n is the only node in the tree that may currently violate a
// B-tree invariant.
func (n *reclaimnode) rebalanceAfterRemove(gap reclaimGapIterator) reclaimGapIterator {
	for {
		if n.nrSegments >= reclaimminDegree-1 {
			return gap
		}
		if n.parent == nil {

			return gap
		}

		if sibling := n.prevSibling(); sibling != nil && sibling.nrSegments >= reclaimminDegree {
			copy(n.keys[1:], n.keys[:n.nrSegments])
			copy(n.values[1:], n.values[:n.nrSegments])
			n.keys[0] = n.parent.keys[n.parentIndex-1]
			n.values[0] = n.parent.values[n.parentIndex-1]
			n.parent.keys[n.parentIndex-1] = sibling.keys[sibling.nrSegments-1]
			n.parent.values[n.parentIndex-1] = sibling.values[sibling.nrSegments-1]
			reclaimSetFunctions{}.ClearValue(&sibling.values[sibling.nrSegments-1])
			if n.hasChildren {
				copy(n.children[1:], n.children[:n.nrSegments+1])
				n.children[0] = sibling.children[sibling.nrSegments]
				sibling.children[sibling.nrSegments] = nil
				n.children[0].parent = n
				n.children[0].parentIndex = 0
				for i := 1; i < n.nrSegments+2; i++ {
					n.children[i].parentIndex = i
				}
			}
			n.nrSegments++
			sibling.nrSegments--

			if reclaimtrackGaps != 0 {
				n.updateMaxGapLocal()
				sibling.updateMaxGapLocal()
			}
			if gap.node == sibling && gap.index == sibling.nrSegments {
				return reclaimGapIterator{n, 0}
			}
			if gap.node == n {
				return reclaimGapIterator{n, gap.index + 1}
			}
			return gap
		}
		if sibling := n.nextSibling(); sibling != nil && sibling.nrSegments >= reclaimminDegree {
			n.keys[n.nrSegments] = n.parent.keys[n.parentIndex]
			n.values[n.nrSegments] = n.parent.values[n.parentIndex]
			n.parent.keys[n.parentIndex] = sibling.keys[0]
			n.parent.values[n.parentIndex] = sibling.values[0]
			copy(sibling.keys[:sibling.nrSegments-1], sibling.keys[1:])
			copy(sibling.values[:sibling.nrSegments-1], sibling.values[1:])
			reclaimSetFunctions{}.ClearValue(&sibling.values[sibling.nrSegments-1])
			if n.hasChildren {
				n.children[n.nrSegments+1] = sibling.children[0]
				copy(sibling.children[:sibling.nrSegments], sibling.children[1:])
				sibling.children[sibling.nrSegments] = nil
				n.children[n.nrSegments+1].parent = n
				n.children[n.nrSegments+1].parentIndex = n.nrSegments + 1
				for i := 0; i < sibling.nrSegments; i++ {
					sibling.children[i].parentIndex = i
				}
			}
			n.nrSegments++
			sibling.nrSegments--

			if reclaimtrackGaps != 0 {
				n.updateMaxGapLocal()
				sibling.updateMaxGapLocal()
			}
			if gap.node == sibling {
				if gap.index == 0 {
					return reclaimGapIterator{n, n.nrSegments}
				}
				return reclaimGapIterator{sibling, gap.index - 1}
			}
			return gap
		}

		p := n.parent
		if p.nrSegments == 1 {

			left, right := p.children[0], p.children[1]
			p.nrSegments = left.nrSegments + right.nrSegments + 1
			p.hasChildren = left.hasChildren
			p.keys[left.nrSegments] = p.keys[0]
			p.values[left.nrSegments] = p.values[0]
			copy(p.keys[:left.nrSegments], left.keys[:left.nrSegments])
			copy(p.values[:left.nrSegments], left.values[:left.nrSegments])
			copy(p.keys[left.nrSegments+1:], right.keys[:right.nrSegments])
			copy(p.values[left.nrSegments+1:], right.values[:right.nrSegments])
			if left.hasChildren {
				copy(p.children[:left.nrSegments+1], left.children[:left.nrSegments+1])
				copy(p.children[left.nrSegments+1:], right.children[:right.nrSegments+1])
				for i := 0; i < p.nrSegments+1; i++ {
					p.children[i].parent = p
					p.children[i].parentIndex = i
				}
			} else {
				p.children[0] = nil
				p.children[1] = nil
			}

			if gap.node == left {
				return reclaimGapIterator{p, gap.index}
			}
			if gap.node == right {
				return reclaimGapIterator{p, gap.index + left.nrSegments + 1}
			}
			return gap
		}
		// Merge n and either sibling, along with the segment separating the
		// two, into whichever of the two nodes comes first. This is the
		// reverse of the non-root splitting case in
		// node.rebalanceBeforeInsert.
		var left, right *reclaimnode
		if n.parentIndex > 0 {
			left = n.prevSibling()
			right = n
		} else {
			left = n
			right = n.nextSibling()
		}

		if gap.node == right {
			gap = reclaimGapIterator{left, gap.index + left.nrSegments + 1}
		}
		left.keys[left.nrSegments] = p.keys[left.parentIndex]
		left.values[left.nrSegments] = p.values[left.parentIndex]
		copy(left.keys[left.nrSegments+1:], right.keys[:right.nrSegments])
		copy(left.values[left.nrSegments+1:], right.values[:right.nrSegments])
		if left.hasChildren {
			copy(left.children[left.nrSegments+1:], right.children[:right.nrSegments+1])
			for i := left.nrSegments + 1; i < left.nrSegments+right.nrSegments+2; i++ {
				left.children[i].parent = left
				left.children[i].parentIndex = i
			}
		}
		left.nrSegments += right.nrSegments + 1
		copy(p.keys[left.parentIndex:], p.keys[left.parentIndex+1:p.nrSegments])
		copy(p.values[left.parentIndex:], p.values[left.parentIndex+1:p.nrSegments])
		reclaimSetFunctions{}.ClearValue(&p.values[p.nrSegments-1])
		copy(p.children[left.parentIndex+1:], p.children[left.parentIndex+2:p.nrSegments+1])
		for i := 0; i < p.nrSegments; i++ {
			p.children[i].parentIndex = i
		}
		p.children[p.nrSegments] = nil
		p.nrSegments--

		if reclaimtrackGaps != 0 {
			left.updateMaxGapLocal()
		}

		n = p
	}
}

// updateMaxGapLeaf updates maxGap bottom-up from the calling leaf until no
// necessary update.
//
// Preconditions: n must be a leaf node, trackGaps must be 1.
func (n *reclaimnode) updateMaxGapLeaf() {
	if n.hasChildren {
		panic(fmt.Sprintf("updateMaxGapLeaf should always be called on leaf node: %v", n))
	}
	max := n.calculateMaxGapLeaf()
	if max == n.maxGap.Get() {

		return
	}
	oldMax := n.maxGap.Get()
	n.maxGap.Set(max)
	if max > oldMax {

		for p := n.parent; p != nil; p = p.parent {
			if p.maxGap.Get() >= max {

				break
			}

			p.maxGap.Set(max)
		}
		return
	}

	for p := n.parent; p != nil; p = p.parent {
		if p.maxGap.Get() > oldMax {

			break
		}

		parentNewMax := p.calculateMaxGapInternal()
		if p.maxGap.Get() == parentNewMax {

			break
		}

		p.maxGap.Set(parentNewMax)
	}
}

// updateMaxGapLocal updates maxGap of the calling node solely with no
// propagation to ancestor nodes.
//
// Precondition: trackGaps must be 1.
func (n *reclaimnode) updateMaxGapLocal() {
	if !n.hasChildren {

		n.maxGap.Set(n.calculateMaxGapLeaf())
	} else {

		n.maxGap.Set(n.calculateMaxGapInternal())
	}
}

// calculateMaxGapLeaf iterates the gaps within a leaf node and calculate the
// max.
//
// Preconditions: n must be a leaf node.
func (n *reclaimnode) calculateMaxGapLeaf() uint64 {
	max := reclaimGapIterator{n, 0}.Range().Length()
	for i := 1; i <= n.nrSegments; i++ {
		if current := (reclaimGapIterator{n, i}).Range().Length(); current > max {
			max = current
		}
	}
	return max
}

// calculateMaxGapInternal iterates children's maxGap within an internal node n
// and calculate the max.
//
// Preconditions: n must be a non-leaf node.
func (n *reclaimnode) calculateMaxGapInternal() uint64 {
	max := n.children[0].maxGap.Get()
	for i := 1; i <= n.nrSegments; i++ {
		if current := n.children[i].maxGap.Get(); current > max {
			max = current
		}
	}
	return max
}

// searchFirstLargeEnoughGap returns the first gap having at least minSize length
// in the subtree rooted by n. If not found, return a terminal gap iterator.
func (n *reclaimnode) searchFirstLargeEnoughGap(minSize uint64) reclaimGapIterator {
	if n.maxGap.Get() < minSize {
		return reclaimGapIterator{}
	}
	if n.hasChildren {
		for i := 0; i <= n.nrSegments; i++ {
			if largeEnoughGap := n.children[i].searchFirstLargeEnoughGap(minSize); largeEnoughGap.Ok() {
				return largeEnoughGap
			}
		}
	} else {
		for i := 0; i <= n.nrSegments; i++ {
			currentGap := reclaimGapIterator{n, i}
			if currentGap.Range().Length() >= minSize {
				return currentGap
			}
		}
	}
	panic(fmt.Sprintf("invalid maxGap in %v", n))
}

// searchLastLargeEnoughGap returns the last gap having at least minSize length
// in the subtree rooted by n. If not found, return a terminal gap iterator.
func (n *reclaimnode) searchLastLargeEnoughGap(minSize uint64) reclaimGapIterator {
	if n.maxGap.Get() < minSize {
		return reclaimGapIterator{}
	}
	if n.hasChildren {
		for i := n.nrSegments; i >= 0; i-- {
			if largeEnoughGap := n.children[i].searchLastLargeEnoughGap(minSize); largeEnoughGap.Ok() {
				return largeEnoughGap
			}
		}
	} else {
		for i := n.nrSegments; i >= 0; i-- {
			currentGap := reclaimGapIterator{n, i}
			if currentGap.Range().Length() >= minSize {
				return currentGap
			}
		}
	}
	panic(fmt.Sprintf("invalid maxGap in %v", n))
}

// A Iterator is conceptually one of:
//
//   - A pointer to a segment in a set; or
//
//   - A terminal iterator, which is a sentinel indicating that the end of
//     iteration has been reached.
//
// Iterators are copyable values and are meaningfully equality-comparable. The
// zero value of Iterator is a terminal iterator.
//
// Unless otherwise specified, any mutation of a set invalidates all existing
// iterators into the set.
type reclaimIterator struct {
	// node is the node containing the iterated segment. If the iterator is
	// terminal, node is nil.
	node *reclaimnode

	// index is the index of the segment in node.keys/values.
	index int
}

// Ok returns true if the iterator is not terminal. All other methods are only
// valid for non-terminal iterators.
func (seg reclaimIterator) Ok() bool {
	return seg.node != nil
}

// Range returns the iterated segment's range key.
func (seg reclaimIterator) Range() __generics_imported0.FileRange {
	return seg.node.keys[seg.index]
}

// Start is equivalent to Range().Start, but should be preferred if only the
// start of the range is needed.
func (seg reclaimIterator) Start() uint64 {
	return seg.node.keys[seg.index].Start
}

// End is equivalent to Range().End, but should be preferred if only the end of
// the range is needed.
func (seg reclaimIterator) End() uint64 {
	return seg.node.keys[seg.index].End
}

// SetRangeUnchecked mutates the iterated segment's range key. This operation
// does not invalidate any iterators.
//
// Preconditions:
// - r.Length() > 0.
// - The new range must not overlap an existing one:
//   - If seg.NextSegment().Ok(), then r.end <= seg.NextSegment().Start().
//   - If seg.PrevSegment().Ok(), then r.start >= seg.PrevSegment().End().
func (seg reclaimIterator) SetRangeUnchecked(r __generics_imported0.FileRange) {
	seg.node.keys[seg.index] = r
}

// SetRange mutates the iterated segment's range key. If the new range would
// cause the iterated segment to overlap another segment, or if the new range
// is invalid, SetRange panics. This operation does not invalidate any
// iterators.
func (seg reclaimIterator) SetRange(r __generics_imported0.FileRange) {
	if r.Length() <= 0 {
		panic(fmt.Sprintf("invalid segment range %v", r))
	}
	if prev := seg.PrevSegment(); prev.Ok() && r.Start < prev.End() {
		panic(fmt.Sprintf("new segment range %v overlaps segment range %v", r, prev.Range()))
	}
	if next := seg.NextSegment(); next.Ok() && r.End > next.Start() {
		panic(fmt.Sprintf("new segment range %v overlaps segment range %v", r, next.Range()))
	}
	seg.SetRangeUnchecked(r)
}

// SetStartUnchecked mutates the iterated segment's start. This operation does
// not invalidate any iterators.
//
// Preconditions: The new start must be valid:
//   - start < seg.End()
//   - If seg.PrevSegment().Ok(), then start >= seg.PrevSegment().End().
func (seg reclaimIterator) SetStartUnchecked(start uint64) {
	seg.node.keys[seg.index].Start = start
}

// SetStart mutates the iterated segment's start. If the new start value would
// cause the iterated segment to overlap another segment, or would result in an
// invalid range, SetStart panics. This operation does not invalidate any
// iterators.
func (seg reclaimIterator) SetStart(start uint64) {
	if start >= seg.End() {
		panic(fmt.Sprintf("new start %v would invalidate segment range %v", start, seg.Range()))
	}
	if prev := seg.PrevSegment(); prev.Ok() && start < prev.End() {
		panic(fmt.Sprintf("new start %v would cause segment range %v to overlap segment range %v", start, seg.Range(), prev.Range()))
	}
	seg.SetStartUnchecked(start)
}

// SetEndUnchecked mutates the iterated segment's end. This operation does not
// invalidate any iterators.
//
// Preconditions: The new end must be valid:
//   - end > seg.Start().
//   - If seg.NextSegment().Ok(), then end <= seg.NextSegment().Start().
func (seg reclaimIterator) SetEndUnchecked(end uint64) {
	seg.node.keys[seg.index].End = end
}

// SetEnd mutates the iterated segment's end. If the new end value would cause
// the iterated segment to overlap another segment, or would result in an
// invalid range, SetEnd panics. This operation does not invalidate any
// iterators.
func (seg reclaimIterator) SetEnd(end uint64) {
	if end <= seg.Start() {
		panic(fmt.Sprintf("new end %v would invalidate segment range %v", end, seg.Range()))
	}
	if next := seg.NextSegment(); next.Ok() && end > next.Start() {
		panic(fmt.Sprintf("new end %v would cause segment range %v to overlap segment range %v", end, seg.Range(), next.Range()))
	}
	seg.SetEndUnchecked(end)
}

// Value returns a copy of the iterated segment's value.
func (seg reclaimIterator) Value() reclaimSetValue {
	return seg.node.values[seg.index]
}

// ValuePtr returns a pointer to the iterated segment's value. The pointer is
// invalidated if the iterator is invalidated. This operation does not
// invalidate any iterators.
func (seg reclaimIterator) ValuePtr() *reclaimSetValue {
	return &seg.node.values[seg.index]
}

// SetValue mutates the iterated segment's value. This operation does not
// invalidate any iterators.
func (seg reclaimIterator) SetValue(val reclaimSetValue) {
	seg.node.values[seg.index] = val
}

// PrevSegment returns the iterated segment's predecessor. If there is no
// preceding segment, PrevSegment returns a terminal iterator.
func (seg reclaimIterator) PrevSegment() reclaimIterator {
	if seg.node.hasChildren {
		return seg.node.children[seg.index].lastSegment()
	}
	if seg.index > 0 {
		return reclaimIterator{seg.node, seg.index - 1}
	}
	if seg.node.parent == nil {
		return reclaimIterator{}
	}
	return reclaimsegmentBeforePosition(seg.node.parent, seg.node.parentIndex)
}

// NextSegment returns the iterated segment's successor. If there is no
// succeeding segment, NextSegment returns a terminal iterator.
func (seg reclaimIterator) NextSegment() reclaimIterator {
	if seg.node.hasChildren {
		return seg.node.children[seg.index+1].firstSegment()
	}
	if seg.index < seg.node.nrSegments-1 {
		return reclaimIterator{seg.node, seg.index + 1}
	}
	if seg.node.parent == nil {
		return reclaimIterator{}
	}
	return reclaimsegmentAfterPosition(seg.node.parent, seg.node.parentIndex)
}

// PrevGap returns the gap immediately before the iterated segment.
func (seg reclaimIterator) PrevGap() reclaimGapIterator {
	if seg.node.hasChildren {

		return seg.node.children[seg.index].lastSegment().NextGap()
	}
	return reclaimGapIterator{seg.node, seg.index}
}

// NextGap returns the gap immediately after the iterated segment.
func (seg reclaimIterator) NextGap() reclaimGapIterator {
	if seg.node.hasChildren {
		return seg.node.children[seg.index+1].firstSegment().PrevGap()
	}
	return reclaimGapIterator{seg.node, seg.index + 1}
}

// PrevNonEmpty returns the iterated segment's predecessor if it is adjacent,
// or the gap before the iterated segment otherwise. If seg.Start() ==
// Functions.MinKey(), PrevNonEmpty will return two terminal iterators.
// Otherwise, exactly one of the iterators returned by PrevNonEmpty will be
// non-terminal.
func (seg reclaimIterator) PrevNonEmpty() (reclaimIterator, reclaimGapIterator) {
	gap := seg.PrevGap()
	if gap.Range().Length() != 0 {
		return reclaimIterator{}, gap
	}
	return gap.PrevSegment(), reclaimGapIterator{}
}

// NextNonEmpty returns the iterated segment's successor if it is adjacent, or
// the gap after the iterated segment otherwise. If seg.End() ==
// Functions.MaxKey(), NextNonEmpty will return two terminal iterators.
// Otherwise, exactly one of the iterators returned by NextNonEmpty will be
// non-terminal.
func (seg reclaimIterator) NextNonEmpty() (reclaimIterator, reclaimGapIterator) {
	gap := seg.NextGap()
	if gap.Range().Length() != 0 {
		return reclaimIterator{}, gap
	}
	return gap.NextSegment(), reclaimGapIterator{}
}

// A GapIterator is conceptually one of:
//
//   - A pointer to a position between two segments, before the first segment, or
//     after the last segment in a set, called a *gap*; or
//
//   - A terminal iterator, which is a sentinel indicating that the end of
//     iteration has been reached.
//
// Note that the gap between two adjacent segments exists (iterators to it are
// non-terminal), but has a length of zero. GapIterator.IsEmpty returns true
// for such gaps. An empty set contains a single gap, spanning the entire range
// of the set's keys.
//
// GapIterators are copyable values and are meaningfully equality-comparable.
// The zero value of GapIterator is a terminal iterator.
//
// Unless otherwise specified, any mutation of a set invalidates all existing
// iterators into the set.
type reclaimGapIterator struct {
	// The representation of a GapIterator is identical to that of an Iterator,
	// except that index corresponds to positions between segments in the same
	// way as for node.children (see comment for node.nrSegments).
	node  *reclaimnode
	index int
}

// Ok returns true if the iterator is not terminal. All other methods are only
// valid for non-terminal iterators.
func (gap reclaimGapIterator) Ok() bool {
	return gap.node != nil
}

// Range returns the range spanned by the iterated gap.
func (gap reclaimGapIterator) Range() __generics_imported0.FileRange {
	return __generics_imported0.FileRange{gap.Start(), gap.End()}
}

// Start is equivalent to Range().Start, but should be preferred if only the
// start of the range is needed.
func (gap reclaimGapIterator) Start() uint64 {
	if ps := gap.PrevSegment(); ps.Ok() {
		return ps.End()
	}
	return reclaimSetFunctions{}.MinKey()
}

// End is equivalent to Range().End, but should be preferred if only the end of
// the range is needed.
func (gap reclaimGapIterator) End() uint64 {
	if ns := gap.NextSegment(); ns.Ok() {
		return ns.Start()
	}
	return reclaimSetFunctions{}.MaxKey()
}

// IsEmpty returns true if the iterated gap is empty (that is, the "gap" is
// between two adjacent segments.)
func (gap reclaimGapIterator) IsEmpty() bool {
	return gap.Range().Length() == 0
}

// PrevSegment returns the segment immediately before the iterated gap. If no
// such segment exists, PrevSegment returns a terminal iterator.
func (gap reclaimGapIterator) PrevSegment() reclaimIterator {
	return reclaimsegmentBeforePosition(gap.node, gap.index)
}

// NextSegment returns the segment immediately after the iterated gap. If no
// such segment exists, NextSegment returns a terminal iterator.
func (gap reclaimGapIterator) NextSegment() reclaimIterator {
	return reclaimsegmentAfterPosition(gap.node, gap.index)
}

// PrevGap returns the iterated gap's predecessor. If no such gap exists,
// PrevGap returns a terminal iterator.
func (gap reclaimGapIterator) PrevGap() reclaimGapIterator {
	seg := gap.PrevSegment()
	if !seg.Ok() {
		return reclaimGapIterator{}
	}
	return seg.PrevGap()
}

// NextGap returns the iterated gap's successor. If no such gap exists, NextGap
// returns a terminal iterator.
func (gap reclaimGapIterator) NextGap() reclaimGapIterator {
	seg := gap.NextSegment()
	if !seg.Ok() {
		return reclaimGapIterator{}
	}
	return seg.NextGap()
}

// NextLargeEnoughGap returns the iterated gap's first next gap with larger
// length than minSize.  If not found, return a terminal gap iterator (does NOT
// include this gap itself).
//
// Precondition: trackGaps must be 1.
func (gap reclaimGapIterator) NextLargeEnoughGap(minSize uint64) reclaimGapIterator {
	if reclaimtrackGaps != 1 {
		panic("set is not tracking gaps")
	}
	if gap.node != nil && gap.node.hasChildren && gap.index == gap.node.nrSegments {

		gap.node = gap.NextSegment().node
		gap.index = 0
		return gap.nextLargeEnoughGapHelper(minSize)
	}
	return gap.nextLargeEnoughGapHelper(minSize)
}

// nextLargeEnoughGapHelper is the helper function used by NextLargeEnoughGap
// to do the real recursions.
//
// Preconditions: gap is NOT the trailing gap of a non-leaf node.
func (gap reclaimGapIterator) nextLargeEnoughGapHelper(minSize uint64) reclaimGapIterator {

	for gap.node != nil &&
		(gap.node.maxGap.Get() < minSize || (!gap.node.hasChildren && gap.index == gap.node.nrSegments)) {
		gap.node, gap.index = gap.node.parent, gap.node.parentIndex
	}

	if gap.node == nil {
		return reclaimGapIterator{}
	}

	gap.index++
	for gap.index <= gap.node.nrSegments {
		if gap.node.hasChildren {
			if largeEnoughGap := gap.node.children[gap.index].searchFirstLargeEnoughGap(minSize); largeEnoughGap.Ok() {
				return largeEnoughGap
			}
		} else {
			if gap.Range().Length() >= minSize {
				return gap
			}
		}
		gap.index++
	}
	gap.node, gap.index = gap.node.parent, gap.node.parentIndex
	if gap.node != nil && gap.index == gap.node.nrSegments {

		gap.node, gap.index = gap.node.parent, gap.node.parentIndex
	}
	return gap.nextLargeEnoughGapHelper(minSize)
}

// PrevLargeEnoughGap returns the iterated gap's first prev gap with larger or
// equal length than minSize.  If not found, return a terminal gap iterator
// (does NOT include this gap itself).
//
// Precondition: trackGaps must be 1.
func (gap reclaimGapIterator) PrevLargeEnoughGap(minSize uint64) reclaimGapIterator {
	if reclaimtrackGaps != 1 {
		panic("set is not tracking gaps")
	}
	if gap.node != nil && gap.node.hasChildren && gap.index == 0 {

		gap.node = gap.PrevSegment().node
		gap.index = gap.node.nrSegments
		return gap.prevLargeEnoughGapHelper(minSize)
	}
	return gap.prevLargeEnoughGapHelper(minSize)
}

// prevLargeEnoughGapHelper is the helper function used by PrevLargeEnoughGap
// to do the real recursions.
//
// Preconditions: gap is NOT the first gap of a non-leaf node.
func (gap reclaimGapIterator) prevLargeEnoughGapHelper(minSize uint64) reclaimGapIterator {

	for gap.node != nil &&
		(gap.node.maxGap.Get() < minSize || (!gap.node.hasChildren && gap.index == 0)) {
		gap.node, gap.index = gap.node.parent, gap.node.parentIndex
	}

	if gap.node == nil {
		return reclaimGapIterator{}
	}

	gap.index--
	for gap.index >= 0 {
		if gap.node.hasChildren {
			if largeEnoughGap := gap.node.children[gap.index].searchLastLargeEnoughGap(minSize); largeEnoughGap.Ok() {
				return largeEnoughGap
			}
		} else {
			if gap.Range().Length() >= minSize {
				return gap
			}
		}
		gap.index--
	}
	gap.node, gap.index = gap.node.parent, gap.node.parentIndex
	if gap.node != nil && gap.index == 0 {

		gap.node, gap.index = gap.node.parent, gap.node.parentIndex
	}
	return gap.prevLargeEnoughGapHelper(minSize)
}

// segmentBeforePosition returns the predecessor segment of the position given
// by n.children[i], which may or may not contain a child. If no such segment
// exists, segmentBeforePosition returns a terminal iterator.
func reclaimsegmentBeforePosition(n *reclaimnode, i int) reclaimIterator {
	for i == 0 {
		if n.parent == nil {
			return reclaimIterator{}
		}
		n, i = n.parent, n.parentIndex
	}
	return reclaimIterator{n, i - 1}
}

// segmentAfterPosition returns the successor segment of the position given by
// n.children[i], which may or may not contain a child. If no such segment
// exists, segmentAfterPosition returns a terminal iterator.
func reclaimsegmentAfterPosition(n *reclaimnode, i int) reclaimIterator {
	for i == n.nrSegments {
		if n.parent == nil {
			return reclaimIterator{}
		}
		n, i = n.parent, n.parentIndex
	}
	return reclaimIterator{n, i}
}

func reclaimzeroValueSlice(slice []reclaimSetValue) {

	for i := range slice {
		reclaimSetFunctions{}.ClearValue(&slice[i])
	}
}

func reclaimzeroNodeSlice(slice []*reclaimnode) {
	for i := range slice {
		slice[i] = nil
	}
}

// String stringifies a Set for debugging.
func (s *reclaimSet) String() string {
	return s.root.String()
}

// String stringifies a node (and all of its children) for debugging.
func (n *reclaimnode) String() string {
	var buf bytes.Buffer
	n.writeDebugString(&buf, "")
	return buf.String()
}

func (n *reclaimnode) writeDebugString(buf *bytes.Buffer, prefix string) {
	if n.hasChildren != (n.nrSegments > 0 && n.children[0] != nil) {
		buf.WriteString(prefix)
		buf.WriteString(fmt.Sprintf("WARNING: inconsistent value of hasChildren: got %v, want %v\n", n.hasChildren, !n.hasChildren))
	}
	for i := 0; i < n.nrSegments; i++ {
		if child := n.children[i]; child != nil {
			cprefix := fmt.Sprintf("%s- % 3d ", prefix, i)
			if child.parent != n || child.parentIndex != i {
				buf.WriteString(cprefix)
				buf.WriteString(fmt.Sprintf("WARNING: inconsistent linkage to parent: got (%p, %d), want (%p, %d)\n", child.parent, child.parentIndex, n, i))
			}
			child.writeDebugString(buf, fmt.Sprintf("%s- % 3d ", prefix, i))
		}
		buf.WriteString(prefix)
		if n.hasChildren {
			if reclaimtrackGaps != 0 {
				buf.WriteString(fmt.Sprintf("- % 3d: %v => %v, maxGap: %d\n", i, n.keys[i], n.values[i], n.maxGap.Get()))
			} else {
				buf.WriteString(fmt.Sprintf("- % 3d: %v => %v\n", i, n.keys[i], n.values[i]))
			}
		} else {
			buf.WriteString(fmt.Sprintf("- % 3d: %v => %v\n", i, n.keys[i], n.values[i]))
		}
	}
	if child := n.children[n.nrSegments]; child != nil {
		child.writeDebugString(buf, fmt.Sprintf("%s- % 3d ", prefix, n.nrSegments))
	}
}

// SegmentDataSlices represents segments from a set as slices of start, end, and
// values. SegmentDataSlices is primarily used as an intermediate representation
// for save/restore and the layout here is optimized for that.
//
// +stateify savable
type reclaimSegmentDataSlices struct {
	Start  []uint64
	End    []uint64
	Values []reclaimSetValue
}

// ExportSortedSlices returns a copy of all segments in the given set, in
// ascending key order.
func (s *reclaimSet) ExportSortedSlices() *reclaimSegmentDataSlices {
	var sds reclaimSegmentDataSlices
	for seg := s.FirstSegment(); seg.Ok(); seg = seg.NextSegment() {
		sds.Start = append(sds.Start, seg.Start())
		sds.End = append(sds.End, seg.End())
		sds.Values = append(sds.Values, seg.Value())
	}
	sds.Start = sds.Start[:len(sds.Start):len(sds.Start)]
	sds.End = sds.End[:len(sds.End):len(sds.End)]
	sds.Values = sds.Values[:len(sds.Values):len(sds.Values)]
	return &sds
}

// ImportSortedSlices initializes the given set from the given slice.
//
// Preconditions:
//   - s must be empty.
//   - sds must represent a valid set (the segments in sds must have valid
//     lengths that do not overlap).
//   - The segments in sds must be sorted in ascending key order.
func (s *reclaimSet) ImportSortedSlices(sds *reclaimSegmentDataSlices) error {
	if !s.IsEmpty() {
		return fmt.Errorf("cannot import into non-empty set %v", s)
	}
	gap := s.FirstGap()
	for i := range sds.Start {
		r := __generics_imported0.FileRange{sds.Start[i], sds.End[i]}
		if !gap.Range().IsSupersetOf(r) {
			return fmt.Errorf("segment overlaps a preceding segment or is incorrectly sorted: [%d, %d) => %v", sds.Start[i], sds.End[i], sds.Values[i])
		}
		gap = s.InsertWithoutMerging(gap, r, sds.Values[i]).NextGap()
	}
	return nil
}

// segmentTestCheck returns an error if s is incorrectly sorted, does not
// contain exactly expectedSegments segments, or contains a segment which
// fails the passed check.
//
// This should be used only for testing, and has been added to this package for
// templating convenience.
func (s *reclaimSet) segmentTestCheck(expectedSegments int, segFunc func(int, __generics_imported0.FileRange, reclaimSetValue) error) error {
	havePrev := false
	prev := uint64(0)
	nrSegments := 0
	for seg := s.FirstSegment(); seg.Ok(); seg = seg.NextSegment() {
		next := seg.Start()
		if havePrev && prev >= next {
			return fmt.Errorf("incorrect order: key %d (segment %d) >= key %d (segment %d)", prev, nrSegments-1, next, nrSegments)
		}
		if segFunc != nil {
			if err := segFunc(nrSegments, seg.Range(), seg.Value()); err != nil {
				return err
			}
		}
		prev = next
		havePrev = true
		nrSegments++
	}
	if nrSegments != expectedSegments {
		return fmt.Errorf("incorrect number of segments: got %d, wanted %d", nrSegments, expectedSegments)
	}
	return nil
}

// countSegments counts the number of segments in the set.
//
// Similar to Check, this should only be used for testing.
func (s *reclaimSet) countSegments() (segments int) {
	for seg := s.FirstSegment(); seg.Ok(); seg = seg.NextSegment() {
		segments++
	}
	return segments
}
func (s *reclaimSet) saveRoot() *reclaimSegmentDataSlices {
	return s.ExportSortedSlices()
}

func (s *reclaimSet) loadRoot(sds *reclaimSegmentDataSlices) {
	if err := s.ImportSortedSlices(sds); err != nil {
		panic(err)
	}
}
