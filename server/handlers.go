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

func (s *Server) handleJoinRedirect(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code")))
	if len(code) != 6 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/sessions/"+code, http.StatusSeeOther)
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	htmx := r.Header.Get("HX-Request") == "true"

	hostName := strings.TrimSpace(r.FormValue("host_name"))
	if hostName == "" {
		formError(w, "name is required", htmx)
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
		formError(w, "at least one card with {blank} is required", htmx)
		return
	}

	sess, host := game.NewSession(hostName, timer, cardTexts)
	s.store.Put(&sess)
	slog.Info("session created", "code", sess.Code, "host", host.Name, "cards", len(cardTexts))

	http.SetCookie(w, &http.Cookie{
		Name:     "participant_token",
		Value:    host.Token,
		Path:     "/",
		MaxAge:   86400 * 30,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	dest := "/sessions/" + sess.Code
	if htmx {
		w.Header().Set("HX-Redirect", dest)
		return
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

func formError(w http.ResponseWriter, msg string, htmx bool) {
	if htmx {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprintf(w, `<p class="form__errors">%s</p>`, msg)
		return
	}
	http.Error(w, msg, http.StatusUnprocessableEntity)
}

func (s *Server) handleShowSession(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	sess, ok := s.store.Get(code)
	if !ok {
		s.renderNotFound(w)
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
	sess, unlock, ok := s.store.Lock(code)
	if !ok {
		s.renderNotFound(w)
		return
	}
	defer unlock()

	r.ParseForm()
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "name is required", http.StatusUnprocessableEntity)
		return
	}

	updated, participant, err := game.Join(sess.Clone(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.store.PutLocked(&updated)
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
	sess, participant, unlock, ok := s.requireHost(w, r)
	if !ok {
		return
	}
	defer unlock()

	updated, err := game.DrawCard(sess.Clone(), participant.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.store.PutLocked(&updated)
	slog.Info("card drawn", "code", sess.Code, "remaining", len(updated.Cards)-len(updated.DrawnCardIDs))

	s.broadcastGameUpdate(&updated)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	sess, participant, unlock, ok := s.requireParticipant(w, r)
	if !ok {
		return
	}
	defer unlock()

	r.ParseForm()

	blanks := r.Form["text"]
	text := strings.Join(blanks, "|||")
	if strings.TrimSpace(text) == "" {
		http.Error(w, "answer is required", http.StatusUnprocessableEntity)
		return
	}

	updated, err := game.Submit(sess.Clone(), participant.ID, text)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.store.PutLocked(&updated)
	slog.Debug("submission received", "code", sess.Code, "participant", participant.Name)

	s.broadcastGameUpdate(&updated)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleAdvance(w http.ResponseWriter, r *http.Request) {
	sess, participant, unlock, ok := s.requireHost(w, r)
	if !ok {
		return
	}
	defer unlock()

	var updated game.Session
	var err error

	clone := sess.Clone()
	switch sess.Status {
	case game.Submitting:
		updated, err = game.AdvanceToVoting(clone, participant.ID)
	case game.Voting:
		updated, err = game.AdvanceToDiscussing(clone, participant.ID)
	default:
		http.Error(w, "cannot advance from this state", http.StatusUnprocessableEntity)
		return
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.store.PutLocked(&updated)
	slog.Info("phase advanced", "code", sess.Code, "status", updated.Status)

	s.broadcastGameUpdate(&updated)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleVote(w http.ResponseWriter, r *http.Request) {
	sess, participant, unlock, ok := s.requireParticipant(w, r)
	if !ok {
		return
	}
	defer unlock()

	r.ParseForm()
	submissionID := r.FormValue("submission_id")

	updated, err := game.CastVote(sess.Clone(), participant.ID, submissionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.store.PutLocked(&updated)
	slog.Debug("vote cast", "code", sess.Code, "participant", participant.Name)

	s.broadcastGameUpdate(&updated)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handlePickWinner(w http.ResponseWriter, r *http.Request) {
	sess, participant, unlock, ok := s.requireHost(w, r)
	if !ok {
		return
	}
	defer unlock()

	r.ParseForm()
	submissionID := r.FormValue("submission_id")

	updated, err := game.PickWinner(sess.Clone(), participant.ID, submissionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.store.PutLocked(&updated)
	slog.Info("winner picked", "code", sess.Code)

	s.broadcastGameUpdate(&updated)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleSkip(w http.ResponseWriter, r *http.Request) {
	sess, participant, unlock, ok := s.requireHost(w, r)
	if !ok {
		return
	}
	defer unlock()

	updated, err := game.SkipCard(sess.Clone(), participant.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.store.PutLocked(&updated)
	slog.Info("card skipped", "code", sess.Code)

	s.broadcastGameUpdate(&updated)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleFinish(w http.ResponseWriter, r *http.Request) {
	sess, participant, unlock, ok := s.requireHost(w, r)
	if !ok {
		return
	}
	defer unlock()

	updated, err := game.Finish(sess.Clone(), participant.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.store.PutLocked(&updated)
	slog.Info("session finished", "code", sess.Code)

	s.broadcastGameUpdate(&updated)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	sess, ok := s.store.Get(code)
	if !ok {
		s.renderNotFound(w)
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

func (s *Server) requireParticipant(w http.ResponseWriter, r *http.Request) (*game.Session, *game.Participant, func(), bool) {
	code := r.PathValue("code")
	sess, unlock, ok := s.store.Lock(code)
	if !ok {
		s.renderNotFound(w)
		return nil, nil, nil, false
	}

	participant := s.participantFromCookie(r, sess)
	if participant == nil {
		unlock()
		http.Error(w, "not a participant", http.StatusForbidden)
		return nil, nil, nil, false
	}

	return sess, participant, unlock, true
}

func (s *Server) requireHost(w http.ResponseWriter, r *http.Request) (*game.Session, *game.Participant, func(), bool) {
	sess, participant, unlock, ok := s.requireParticipant(w, r)
	if !ok {
		return nil, nil, nil, false
	}

	if !participant.Host {
		unlock()
		http.Error(w, "not the host", http.StatusForbidden)
		return nil, nil, nil, false
	}

	return sess, participant, unlock, true
}
