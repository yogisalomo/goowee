package core

// List is a keyed list that records what changed (#140): an append, an
// update of one item, a removal or a move. h.ForList applies each change to
// the rows it rendered (mounting one row, setting one row's item signal,
// removing or moving one row) instead of comparing the whole list again, as
// For does with a *Signal[[]T].
//
//	todos := core.NewList(func(t Todo) int { return t.ID })
//	todos.Append(Todo{ID: 1, Text: "milk"})
//	todos.Update(1, func(t *Todo) { t.Done = true })
//	todos.Remove(1)
//
// A List is a SignalAccessor: a Computed or a binding that lists it as a
// dependency follows every change. Items returns the current items.
type List[K comparable, T any] struct {
	key     func(T) K
	items   []T
	version *Signal[uint64] // bumped by every change; what subscribers watch
	log     []ListOp[K, T]  // the most recent changes, for renderers catching up
	logBase uint64          // the version of log[0]
}

// ListOpKind is what a ListOp did.
type ListOpKind uint8

const (
	ListInsert  ListOpKind = iota + 1 // Item inserted at Index
	ListUpdate                        // the item at Index replaced by Item
	ListRemove                        // the item at Index removed
	ListMove                          // the item at Index moved to To
	ListReplace                       // the whole list replaced (Set)
)

// ListOp is one recorded change. Indexes are positions in the list as it
// was just before the change.
type ListOp[K comparable, T any] struct {
	Kind  ListOpKind
	Index int
	To    int
	Key   K
	Item  T
}

// listLog is how many changes a List keeps. A renderer further behind than
// that renders the whole list again.
const listLog = 1024

// NewList creates an empty list whose items are told apart by key.
func NewList[K comparable, T any](key func(T) K, items ...T) *List[K, T] {
	return &List[K, T]{key: key, items: append([]T(nil), items...), version: NewSignal(uint64(0))}
}

// Key is the list's key function.
func (l *List[K, T]) Key(item T) K { return l.key(item) }

// Items returns the current items. The slice is the list's own: read it,
// don't keep it past the next change, and don't modify it.
func (l *List[K, T]) Items() []T {
	l.version.Get() // a read inside a Computed or effect, like Signal.Get
	return l.items
}

// Len is the number of items.
func (l *List[K, T]) Len() int { return len(l.items) }

// Index is key's position, or -1.
func (l *List[K, T]) Index(key K) int {
	for i, it := range l.items {
		if l.key(it) == key {
			return i
		}
	}
	return -1
}

// Get returns key's item.
func (l *List[K, T]) Get(key K) (T, bool) {
	if i := l.Index(key); i >= 0 {
		return l.items[i], true
	}
	var zero T
	return zero, false
}

// Append adds item at the end.
func (l *List[K, T]) Append(item T) { l.Insert(len(l.items), item) }

// Insert adds item at index (clamped to the list).
func (l *List[K, T]) Insert(index int, item T) {
	index = max(0, min(index, len(l.items)))
	var zero T
	l.items = append(l.items, zero)
	copy(l.items[index+1:], l.items[index:])
	l.items[index] = item
	l.record(ListOp[K, T]{Kind: ListInsert, Index: index, Key: l.key(item), Item: item})
}

// Update changes key's item through fn. It reports whether the list has key.
// An update that changes the key replaces the item (its row is mounted
// again).
func (l *List[K, T]) Update(key K, fn func(item *T)) bool {
	i := l.Index(key)
	if i < 0 {
		return false
	}
	item := l.items[i]
	fn(&item)
	if l.key(item) != key {
		l.items[i] = item
		l.record(ListOp[K, T]{Kind: ListReplace})
		return true
	}
	l.items[i] = item
	l.record(ListOp[K, T]{Kind: ListUpdate, Index: i, Key: key, Item: item})
	return true
}

// Remove removes key's item and reports whether it was there.
func (l *List[K, T]) Remove(key K) bool {
	i := l.Index(key)
	if i < 0 {
		return false
	}
	copy(l.items[i:], l.items[i+1:])
	var zero T
	l.items[len(l.items)-1] = zero
	l.items = l.items[:len(l.items)-1]
	l.record(ListOp[K, T]{Kind: ListRemove, Index: i, Key: key})
	return true
}

// Move moves key's item to index (clamped) and reports whether the list has
// key.
func (l *List[K, T]) Move(key K, index int) bool {
	from := l.Index(key)
	if from < 0 {
		return false
	}
	index = max(0, min(index, len(l.items)-1))
	if index == from {
		return true
	}
	item := l.items[from]
	if from < index {
		copy(l.items[from:index], l.items[from+1:index+1])
	} else {
		copy(l.items[index+1:from+1], l.items[index:from])
	}
	l.items[index] = item
	l.record(ListOp[K, T]{Kind: ListMove, Index: from, To: index, Key: key})
	return true
}

// Set replaces the whole list. Renderers compare the new items with their
// rows by key, as For does.
func (l *List[K, T]) Set(items []T) {
	l.items = append(l.items[:0:0], items...)
	l.record(ListOp[K, T]{Kind: ListReplace})
}

func (l *List[K, T]) record(op ListOp[K, T]) {
	if len(l.log) == listLog {
		// Drop the older half: a renderer that far behind renders everything.
		n := copy(l.log, l.log[listLog/2:])
		clear(l.log[n:])
		l.log = l.log[:n]
		l.logBase += listLog / 2
	}
	l.log = append(l.log, op)
	l.version.Set(l.version.Peek() + 1)
}

// Version counts the changes so far.
func (l *List[K, T]) Version() uint64 { return l.version.Peek() }

// Changes returns the changes after version since (oldest first), and false
// when they are no longer all kept: render the whole list then. The slice is
// the list's own; don't keep it.
func (l *List[K, T]) Changes(since uint64) ([]ListOp[K, T], bool) {
	now := l.version.Peek()
	if since == now {
		return nil, true
	}
	if since < l.logBase || since > now {
		return nil, false
	}
	return l.log[since-l.logBase:], true
}

// Value is the items as an any, for SignalAccessor.
func (l *List[K, T]) Value() any { return l.Items() }

// Subscribe calls fn after each change (once per Batch), for SignalAccessor.
func (l *List[K, T]) Subscribe(fn func()) func() { return l.version.Subscribe(fn) }

var _ SignalAccessor = (*List[int, int])(nil)
