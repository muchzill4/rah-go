package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/muchzill4/rah-go/game"
)

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	s.render(w, "home", map[string]any{
		"DefaultCards": strings.Join(game.DefaultCards, "\n"),
	})
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	hostName := strings.TrimSpace(r.FormValue("host_name"))
	if hostName == "" {
		http.Error(w, "name is required", http.StatusUnprocessableEntity)
		return
	}

	timerStr := r.FormValue("timer")
	timer, err := strconv.Atoi(timerStr)
	if err != nil || timer < 10 || timer > 300 {
		timer = 60
	}

	cardsRaw := r.FormValue("cards")
	var cardTexts []string
	for _, line := range strings.Split(cardsRaw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		normalized := game.NormalizeCardText(line)
		if game.BlankCount(normalized) == 0 {
			continue
		}
		cardTexts = append(cardTexts, normalized)
	}
	if len(cardTexts) == 0 {
		http.Error(w, "at least one card with {blank} is required", http.StatusUnprocessableEntity)
		return
	}

	sess, host := game.NewSession(hostName, timer, cardTexts)
	s.putSession(&sess)
	slog.Info("session created", "code", sess.Code, "host", host.Name, "cards", len(cardTexts))

	http.SetCookie(w, &http.Cookie{
		Name:     "participant_token",
		Value:    host.Token,
		Path:     "/",
		MaxAge:   86400 * 30,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/sessions/"+sess.Code, http.StatusSeeOther)
}

func (s *Server) handleShowSession(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	sess, ok := s.getSession(code)
	if !ok {
		http.NotFound(w, r)
		return
	}

	participant := s.participantFromCookie(r, sess)

	s.render(w, "session", map[string]any{
		"Session":     sess,
		"Participant": participant,
	})
}

func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	sess, ok := s.getSession(code)
	if !ok {
		http.NotFound(w, r)
		return
	}

	r.ParseForm()
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "name is required", http.StatusUnprocessableEntity)
		return
	}

	updated, participant, err := game.Join(*sess, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.putSession(&updated)
	slog.Info("participant joined", "code", code, "name", name)

	http.SetCookie(w, &http.Cookie{
		Name:     "participant_token",
		Value:    participant.Token,
		Path:     "/",
		MaxAge:   86400 * 30,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	s.broadcastGameUpdate(&updated)
	http.Redirect(w, r, "/sessions/"+code, http.StatusSeeOther)
}

func (s *Server) handleDraw(w http.ResponseWriter, r *http.Request) {
	sess, participant, ok := s.requireHost(w, r)
	if !ok {
		return
	}

	updated, err := game.DrawCard(*sess, participant.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.putSession(&updated)
	slog.Info("card drawn", "code", sess.Code, "remaining", len(updated.Cards)-len(updated.DrawnCardIDs))

	s.broadcastGameUpdate(&updated)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	sess, participant, ok := s.requireParticipant(w, r)
	if !ok {
		return
	}

	r.ParseForm()

	blanks := r.Form["text"]
	text := strings.Join(blanks, "|||")
	if strings.TrimSpace(text) == "" {
		http.Error(w, "answer is required", http.StatusUnprocessableEntity)
		return
	}

	updated, err := game.Submit(*sess, participant.ID, text)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.putSession(&updated)
	slog.Debug("submission received", "code", sess.Code, "participant", participant.Name)

	s.broadcastGameUpdate(&updated)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleAdvance(w http.ResponseWriter, r *http.Request) {
	sess, participant, ok := s.requireHost(w, r)
	if !ok {
		return
	}

	var updated game.Session
	var err error

	switch sess.Status {
	case game.Submitting:
		updated, err = game.AdvanceToVoting(*sess, participant.ID)
	case game.Voting:
		updated, err = game.AdvanceToDiscussing(*sess, participant.ID)
	default:
		http.Error(w, "cannot advance from this state", http.StatusUnprocessableEntity)
		return
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.putSession(&updated)
	slog.Info("phase advanced", "code", sess.Code, "status", updated.Status)

	s.broadcastGameUpdate(&updated)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleVote(w http.ResponseWriter, r *http.Request) {
	sess, participant, ok := s.requireParticipant(w, r)
	if !ok {
		return
	}

	r.ParseForm()
	submissionID := r.FormValue("submission_id")

	updated, err := game.CastVote(*sess, participant.ID, submissionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.putSession(&updated)
	slog.Debug("vote cast", "code", sess.Code, "participant", participant.Name)

	s.broadcastGameUpdate(&updated)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handlePickWinner(w http.ResponseWriter, r *http.Request) {
	sess, participant, ok := s.requireHost(w, r)
	if !ok {
		return
	}

	r.ParseForm()
	submissionID := r.FormValue("submission_id")

	updated, err := game.PickWinner(*sess, participant.ID, submissionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.putSession(&updated)
	slog.Info("winner picked", "code", sess.Code)

	s.broadcastGameUpdate(&updated)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleSkip(w http.ResponseWriter, r *http.Request) {
	sess, participant, ok := s.requireHost(w, r)
	if !ok {
		return
	}

	updated, err := game.SkipCard(*sess, participant.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.putSession(&updated)
	slog.Info("card skipped", "code", sess.Code)

	s.broadcastGameUpdate(&updated)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleFinish(w http.ResponseWriter, r *http.Request) {
	sess, participant, ok := s.requireHost(w, r)
	if !ok {
		return
	}

	updated, err := game.Finish(*sess, participant.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.putSession(&updated)
	slog.Info("session finished", "code", sess.Code)

	s.broadcastGameUpdate(&updated)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	sess, ok := s.getSession(code)
	if !ok {
		http.NotFound(w, r)
		return
	}

	participant := s.participantFromCookie(r, sess)
	if participant == nil {
		http.Error(w, "not a participant", http.StatusForbidden)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := s.broker.Subscribe(participant.ID)
	defer s.broker.Unsubscribe(participant.ID)
	slog.Debug("sse connected", "code", code, "participant", participant.Name)

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: game-update\n")
			for _, line := range strings.Split(msg, "\n") {
				fmt.Fprintf(w, "data: %s\n", line)
			}
			fmt.Fprintf(w, "\n")
			flusher.Flush()
		}
	}
}

func (s *Server) broadcastGameUpdate(sess *game.Session) {
	for _, p := range sess.Participants {
		html := s.renderPartial("game", map[string]any{
			"Session":     sess,
			"Participant": &p,
		})
		s.broker.Send(p.ID, html)
	}
}

func (s *Server) requireParticipant(w http.ResponseWriter, r *http.Request) (*game.Session, *game.Participant, bool) {
	code := r.PathValue("code")
	sess, ok := s.getSession(code)
	if !ok {
		http.NotFound(w, r)
		return nil, nil, false
	}

	participant := s.participantFromCookie(r, sess)
	if participant == nil {
		http.Error(w, "not a participant", http.StatusForbidden)
		return nil, nil, false
	}

	return sess, participant, true
}

func (s *Server) requireHost(w http.ResponseWriter, r *http.Request) (*game.Session, *game.Participant, bool) {
	sess, participant, ok := s.requireParticipant(w, r)
	if !ok {
		return nil, nil, false
	}

	if !participant.Host {
		http.Error(w, "not the host", http.StatusForbidden)
		return nil, nil, false
	}

	return sess, participant, true
}
