package agent

import (
	"sync"
	"time"
)

// Session represents an agent session tied to a chat session.
type Session struct {
	ID         string // matches session_id from chat_sessions table
	Mode       string // "chat" or "agent"
	Memory     Memory
	LastAccess time.Time
}

// SessionStore is the interface for managing agent sessions.
type SessionStore interface {
	Get(id string) *Session
	GetOrCreate(id string) *Session
	Delete(id string)
}

// InMemorySessionStore is a thread-safe in-memory session store with TTL-based cleanup.
type InMemorySessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	ttl      time.Duration
	stopCh   chan struct{}
}

// NewInMemorySessionStore creates a new store that purges sessions idle for longer
// than the given TTL. If ttl <= 0, defaults to 30 minutes.
func NewInMemorySessionStore(ttl time.Duration) *InMemorySessionStore {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	store := &InMemorySessionStore{
		sessions: make(map[string]*Session),
		ttl:      ttl,
		stopCh:   make(chan struct{}),
	}
	go store.cleanup()
	return store
}

func (s *InMemorySessionStore) Get(id string) *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil
	}
	sess.LastAccess = time.Now()
	return sess
}

func (s *InMemorySessionStore) GetOrCreate(id string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[id]; ok {
		sess.LastAccess = time.Now()
		return sess
	}
	sess := &Session{
		ID:         id,
		Mode:       "chat",
		Memory:     NewBufferMemory(20),
		LastAccess: time.Now(),
	}
	s.sessions[id] = sess
	return sess
}

func (s *InMemorySessionStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

// Stop terminates the background cleanup goroutine.
func (s *InMemorySessionStore) Stop() {
	close(s.stopCh)
}

func (s *InMemorySessionStore) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.purgeExpired()
		}
	}
}

func (s *InMemorySessionStore) purgeExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for id, sess := range s.sessions {
		if now.Sub(sess.LastAccess) > s.ttl {
			delete(s.sessions, id)
		}
	}
}
