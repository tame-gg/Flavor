package secret

import (
	"context"
	"sync"
)

type MemoryStore struct {
	mu   sync.RWMutex
	data map[Ref]string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[Ref]string)}
}

func (m *MemoryStore) Get(_ context.Context, ref Ref) (Secret, error) {
	if _, err := ParseRef(string(ref)); err != nil {
		return Secret{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[ref]
	if !ok {
		return Secret{}, ErrNotFound
	}
	return New(v), nil
}

func (m *MemoryStore) Set(_ context.Context, ref Ref, secret Secret) error {
	if _, err := ParseRef(string(ref)); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[ref] = secret.Reveal()
	return nil
}

func (m *MemoryStore) Delete(_ context.Context, ref Ref) error {
	if _, err := ParseRef(string(ref)); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, ref)
	return nil
}

func (m *MemoryStore) Status(context.Context) Status {
	return Status{
		Backend: BackendMemory,
		State:   StateMemory,
		Detail:  "process-local non-persistent store",
	}
}
