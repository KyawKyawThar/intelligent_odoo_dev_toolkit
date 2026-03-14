// Package ringbuf provides a thread-safe fixed-size circular buffer for
// agent error events. When the buffer is full, Push silently overwrites the
// oldest entry so the agent never blocks or allocates unbounded memory.
package ringbuf

import "sync"

// RingBuffer is a thread-safe circular buffer of any comparable type.
// Cap is set at construction time and never changes.
type RingBuffer[T any] struct {
	items []T
	head  int // next write position (mod cap)
	count int // number of valid entries (0 ≤ count ≤ cap)
	mu    sync.Mutex
}

// New returns a RingBuffer with the given capacity.
// Panics if cap ≤ 0.
func New[T any](capacity int) *RingBuffer[T] {
	if capacity <= 0 {
		panic("ringbuf: capacity must be > 0")
	}
	return &RingBuffer[T]{items: make([]T, capacity)}
}

// Cap returns the fixed capacity of the buffer.
func (r *RingBuffer[T]) Cap() int { return len(r.items) }

// Len returns the number of entries currently held.
func (r *RingBuffer[T]) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count
}

// Push appends item. If the buffer is full the oldest entry is overwritten.
func (r *RingBuffer[T]) Push(item T) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.items[r.head] = item
	r.head = (r.head + 1) % len(r.items)

	if r.count < len(r.items) {
		r.count++
	}
	// When count == cap, head wrapped past tail — oldest entry is overwritten.
}

// DrainAll atomically removes and returns all current entries, oldest first.
// The buffer is empty after this call.
func (r *RingBuffer[T]) DrainAll() []T {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.count == 0 {
		return nil
	}

	out := make([]T, r.count)
	// tail is the index of the oldest entry.
	tail := (r.head - r.count + len(r.items)) % len(r.items)
	for i := range out {
		out[i] = r.items[(tail+i)%len(r.items)]
	}

	r.count = 0
	r.head = 0
	return out
}

// Peek returns a snapshot of all current entries without removing them.
// Useful for monitoring / metrics.
func (r *RingBuffer[T]) Peek() []T {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.count == 0 {
		return nil
	}

	out := make([]T, r.count)
	tail := (r.head - r.count + len(r.items)) % len(r.items)
	for i := range out {
		out[i] = r.items[(tail+i)%len(r.items)]
	}
	return out
}
