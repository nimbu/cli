package realtime

// DefaultDedupeCapacity is the number of event ids remembered per watch.
const DefaultDedupeCapacity = 4096

// Deduper remembers recently seen event ids in a bounded FIFO set.
// Delivery is at-least-once, so repeated ids must be dropped.
type Deduper struct {
	capacity int
	seen     map[string]struct{}
	order    []string
}

// NewDeduper creates a Deduper holding at most capacity ids.
func NewDeduper(capacity int) *Deduper {
	if capacity <= 0 {
		capacity = DefaultDedupeCapacity
	}
	return &Deduper{
		capacity: capacity,
		seen:     make(map[string]struct{}, capacity),
		order:    make([]string, 0, capacity),
	}
}

// Seen records an id and reports whether it was already known.
// An empty id is never treated as a duplicate.
func (d *Deduper) Seen(id string) bool {
	if d == nil || id == "" {
		return false
	}
	if _, ok := d.seen[id]; ok {
		return true
	}
	if len(d.order) >= d.capacity {
		oldest := d.order[0]
		d.order = d.order[1:]
		delete(d.seen, oldest)
	}
	d.seen[id] = struct{}{}
	d.order = append(d.order, id)
	return false
}
