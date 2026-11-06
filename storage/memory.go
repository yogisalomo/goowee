//go:build !(js && wasm)

package storage

// The native build keeps each area in memory, process-wide.

func (s *Store) get(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.mem[key]
	return v, ok
}

func (s *Store) set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mem == nil {
		s.mem = map[string]string{}
	}
	s.mem[key] = value
	return nil
}

func (s *Store) remove(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.mem, key)
}

func (s *Store) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mem = nil
}

func (s *Store) keys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]string, 0, len(s.mem))
	for k := range s.mem {
		keys = append(keys, k)
	}
	return keys
}
