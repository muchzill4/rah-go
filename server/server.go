package server

import (
	"net/http"
	"sync"

	"github.com/muchzill4/rah-go/game"
)

type Server struct {
	mu       sync.RWMutex
	sessions map[string]*game.Session
	broker   *Broker
	mux      *http.ServeMux
}

func New() *Server {
	s := &Server{
		sessions: make(map[string]*game.Session),
		broker:   NewBroker(),
		mux:      http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
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
