// Package safe provides a generic thread-safe key-value store.
package safe

import (
	"iter"
	"maps"
	"sync"
)

// Store is a thread-safe generic key-value map.
// Reads use a shared lock so concurrent lookups never block each other.
type Store[Object any] struct {
	mtx  sync.RWMutex
	data map[string]Object
}

// NewStore returns an empty, ready-to-use Store.
func NewStore[Object any]() *Store[Object] {
	return &Store[Object]{
		data: make(map[string]Object),
	}
}

// Put inserts or replaces the value for key.
func (s *Store[Object]) Put(key string, value Object) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.data[key] = value
}

// Get returns the value for key and whether it was present.
func (s *Store[Object]) Get(key string) (Object, bool) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	value, ok := s.data[key]

	return value, ok
}

// Delete removes the entry for key. It is a no-op when key is absent.
func (s *Store[Object]) Delete(key string) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	delete(s.data, key)
}

// Iter returns a range-over-func iterator over a point-in-time snapshot of the
// store. Mutations made after Iter is called are not visible to the iterator.
func (s *Store[Object]) Iter() iter.Seq2[string, Object] {
	return func(yield func(string, Object) bool) {
		s.mtx.RLock()

		snapshot := make(map[string]Object, len(s.data))

		maps.Copy(snapshot, s.data)

		s.mtx.RUnlock()

		for key, value := range snapshot {
			if !yield(key, value) {
				break
			}
		}
	}
}
