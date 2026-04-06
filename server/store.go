package server

import (
	"log/slog"
	"sync"
	"time"

	"github.com/muchzill4/rah-go/game"
)

const sessionMaxAge = 24 * time.Hour

type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*game.Session
}

func NewSessionStore() *SessionStore {
	st := &SessionStore{
		sessions: make(map[string]*game.Session),
	}
	go st.cleanupLoop()
	return st
}

func (st *SessionStore) Get(code string) (*game.Session, bool) {
	st.mu.RLock()
	defer st.mu.RUnlock()
	sess, ok := st.sessions[code]
	return sess, ok
}

func (st *SessionStore) Put(sess *game.Session) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.sessions[sess.Code] = sess
}

// Lock acquires an exclusive lock and returns the session.
// The caller must defer the returned unlock function.
func (st *SessionStore) Lock(code string) (*game.Session, func(), bool) {
	st.mu.Lock()
	sess, ok := st.sessions[code]
	if !ok {
		st.mu.Unlock()
		return nil, nil, false
	}
	return sess, st.mu.Unlock, true
}

// PutLocked stores the session. Caller must hold the lock from Lock.
func (st *SessionStore) PutLocked(sess *game.Session) {
	st.sessions[sess.Code] = sess
}

func (st *SessionStore) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		st.mu.Lock()
		before := len(st.sessions)
		for code, sess := range st.sessions {
			if time.Since(sess.CreatedAt) > sessionMaxAge {
				delete(st.sessions, code)
			}
		}
		after := len(st.sessions)
		st.mu.Unlock()

		if removed := before - after; removed > 0 {
			slog.Info("cleaned up expired sessions", "count", removed)
		}
	}
}
