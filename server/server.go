package server

import (
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/muchzill4/rah-go/game"
)

//go:embed static/*
var staticFS embed.FS

const sessionMaxAge = 24 * time.Hour

type Server struct {
	mu       sync.RWMutex
	sessions map[string]*game.Session
	broker   *Broker
	mux      *http.ServeMux
	handler  http.Handler
}

func New() *Server {
	s := &Server{
		sessions: make(map[string]*game.Session),
		broker:   NewBroker(),
		mux:      http.NewServeMux(),
	}
	s.routes()
	s.handler = requestLogger(s.mux)
	go s.cleanupLoop()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

func (s *Server) routes() {
	staticContent, _ := fs.Sub(staticFS, "static")
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticContent))))
	s.mux.HandleFunc("GET /", s.handleHome)
	s.mux.HandleFunc("POST /sessions", s.handleCreateSession)
	s.mux.HandleFunc("GET /sessions/{code}", s.handleShowSession)
	s.mux.HandleFunc("POST /sessions/{code}/participants", s.handleJoin)
	s.mux.HandleFunc("POST /sessions/{code}/draw", s.handleDraw)
	s.mux.HandleFunc("POST /sessions/{code}/submissions", s.handleSubmit)
	s.mux.HandleFunc("POST /sessions/{code}/advance", s.handleAdvance)
	s.mux.HandleFunc("POST /sessions/{code}/votes", s.handleVote)
	s.mux.HandleFunc("POST /sessions/{code}/winners", s.handlePickWinner)
	s.mux.HandleFunc("POST /sessions/{code}/skip", s.handleSkip)
	s.mux.HandleFunc("POST /sessions/{code}/summary", s.handleFinish)
	s.mux.HandleFunc("GET /sessions/{code}/events", s.handleSSE)
}

func (s *Server) getSession(code string) (*game.Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[code]
	return sess, ok
}

func (s *Server) putSession(sess *game.Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sess.Code] = sess
}

func (s *Server) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		before := len(s.sessions)
		for code, sess := range s.sessions {
			if time.Since(sess.CreatedAt) > sessionMaxAge {
				delete(s.sessions, code)
			}
		}
		after := len(s.sessions)
		s.mu.Unlock()

		if removed := before - after; removed > 0 {
			slog.Info("cleaned up expired sessions", "count", removed)
		}
	}
}

func (s *Server) participantFromCookie(r *http.Request, sess *game.Session) *game.Participant {
	cookie, err := r.Cookie("participant_token")
	if err != nil {
		return nil
	}
	for i := range sess.Participants {
		if sess.Participants[i].Token == cookie.Value {
			return &sess.Participants[i]
		}
	}
	return nil
}
