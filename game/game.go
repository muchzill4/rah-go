package game

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"time"
)

var (
	ErrWrongStatus      = errors.New("wrong session status")
	ErrNotHost          = errors.New("not the host")
	ErrAlreadySubmitted = errors.New("already submitted for this card")
	ErrAlreadyVoted     = errors.New("already voted this round")
)

type Status int

const (
	Lobby Status = iota
	Submitting
	Voting
	Discussing
	Finished
)

type Card struct {
	ID   string
	Text string
}

type Participant struct {
	ID    string
	Name  string
	Token string
	Host  bool
}

type Submission struct {
	ID            string
	ParticipantID string
	CardID        string
	Text          string
	Winner        bool
}

type Vote struct {
	ParticipantID string
	SubmissionID  string
}

type Session struct {
	Code                   string
	Status                 Status
	SubmissionTimerSeconds int
	SubmissionDeadline     time.Time
	CurrentCard            *Card
	Cards                  []Card
	DrawnCardIDs           map[string]bool
	Participants           []Participant
	Submissions            []Submission
	Votes                  []Vote
}

func NewSession(hostName string, timerSeconds int, cardTexts []string) (Session, Participant) {
	cards := make([]Card, len(cardTexts))
	for i, text := range cardTexts {
		cards[i] = Card{
			ID:   fmt.Sprintf("card-%d", i+1),
			Text: text,
		}
	}

	host := Participant{
		ID:    "p-1",
		Name:  hostName,
		Token: generateToken(),
		Host:  true,
	}

	s := Session{
		Code:                   generateCode(),
		Status:                 Lobby,
		SubmissionTimerSeconds: timerSeconds,
		Cards:                  cards,
		DrawnCardIDs:           make(map[string]bool),
		Participants:           []Participant{host},
	}

	return s, host
}

func Join(s Session, name string) (Session, Participant, error) {
	if s.Status != Lobby {
		return s, Participant{}, ErrWrongStatus
	}

	p := Participant{
		ID:    fmt.Sprintf("p-%d", len(s.Participants)+1),
		Name:  name,
		Token: generateToken(),
	}
	s.Participants = append(s.Participants, p)

	return s, p, nil
}

func DrawCard(s Session, participantID string) (Session, error) {
	if s.Status != Lobby && s.Status != Discussing {
		return s, ErrWrongStatus
	}
	if !isHost(s, participantID) {
		return s, ErrNotHost
	}

	undrawn := undrawnCards(s)
	if len(undrawn) == 0 {
		s.Status = Finished
		return s, nil
	}

	n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(undrawn))))
	card := undrawn[n.Int64()]
	s.CurrentCard = &card
	s.DrawnCardIDs[card.ID] = true
	s.Status = Submitting
	s.SubmissionDeadline = time.Now().Add(time.Duration(s.SubmissionTimerSeconds) * time.Second)

	return s, nil
}

func AllSubmitted(s Session) bool {
	submitted := make(map[string]bool)
	for _, sub := range s.Submissions {
		if sub.CardID == s.CurrentCard.ID {
			submitted[sub.ParticipantID] = true
		}
	}
	return len(submitted) == len(s.Participants)
}

func AllVoted(s Session) bool {
	voted := make(map[string]bool)
	for _, v := range s.Votes {
		if hasVotedThisRound(s, v) {
			voted[v.ParticipantID] = true
		}
	}
	return len(voted) == len(s.Participants)
}

func PickWinner(s Session, participantID string, submissionID string) (Session, error) {
	if s.Status != Discussing {
		return s, ErrWrongStatus
	}
	if !isHost(s, participantID) {
		return s, ErrNotHost
	}

	for i, sub := range s.Submissions {
		if sub.ID == submissionID {
			s.Submissions[i].Winner = true
			break
		}
	}

	return s, nil
}

func SkipCard(s Session, participantID string) (Session, error) {
	if s.Status != Submitting {
		return s, ErrWrongStatus
	}
	if !isHost(s, participantID) {
		return s, ErrNotHost
	}

	s.Status = Discussing
	return s, nil
}

func Finish(s Session, participantID string) (Session, error) {
	if !isHost(s, participantID) {
		return s, ErrNotHost
	}

	s.Status = Finished
	return s, nil
}

func AdvanceToVoting(s Session, participantID string) (Session, error) {
	if s.Status != Submitting {
		return s, ErrWrongStatus
	}
	if !isHost(s, participantID) {
		return s, ErrNotHost
	}

	s.Status = Voting
	return s, nil
}

func AdvanceToDiscussing(s Session, participantID string) (Session, error) {
	if s.Status != Voting {
		return s, ErrWrongStatus
	}
	if !isHost(s, participantID) {
		return s, ErrNotHost
	}

	s.Status = Discussing
	return s, nil
}

// WinningSubmission returns the clear winner (if any) and tied submissions.
// If there's a clear winner: winner is non-nil, tied is empty.
// If there's a tie: winner is nil, tied contains the tied submissions.
// If no votes: both are nil/empty.
func WinningSubmission(s Session) (*Submission, []Submission) {
	currentSubs := currentSubmissions(s)
	if len(currentSubs) == 0 {
		return nil, nil
	}

	counts := make(map[string]int)
	for _, v := range s.Votes {
		counts[v.SubmissionID]++
	}

	maxVotes := 0
	for _, sub := range currentSubs {
		if c := counts[sub.ID]; c > maxVotes {
			maxVotes = c
		}
	}

	if maxVotes == 0 {
		return nil, nil
	}

	var top []Submission
	for _, sub := range currentSubs {
		if counts[sub.ID] == maxVotes {
			top = append(top, sub)
		}
	}

	if len(top) == 1 {
		return &top[0], nil
	}
	return nil, top
}

func CastVote(s Session, participantID string, submissionID string) (Session, error) {
	if s.Status != Voting {
		return s, ErrWrongStatus
	}
	for _, v := range s.Votes {
		if v.ParticipantID == participantID && hasVotedThisRound(s, v) {
			return s, ErrAlreadyVoted
		}
	}

	s.Votes = append(s.Votes, Vote{
		ParticipantID: participantID,
		SubmissionID:  submissionID,
	})

	return s, nil
}

func Submit(s Session, participantID string, text string) (Session, error) {
	if s.Status != Submitting {
		return s, ErrWrongStatus
	}
	for _, sub := range s.Submissions {
		if sub.ParticipantID == participantID && sub.CardID == s.CurrentCard.ID {
			return s, ErrAlreadySubmitted
		}
	}

	sub := Submission{
		ID:            fmt.Sprintf("sub-%d", len(s.Submissions)+1),
		ParticipantID: participantID,
		CardID:        s.CurrentCard.ID,
		Text:          text,
	}
	s.Submissions = append(s.Submissions, sub)

	return s, nil
}

func isHost(s Session, participantID string) bool {
	for _, p := range s.Participants {
		if p.ID == participantID {
			return p.Host
		}
	}
	return false
}

func undrawnCards(s Session) []Card {
	var cards []Card
	for _, c := range s.Cards {
		if !s.DrawnCardIDs[c.ID] {
			cards = append(cards, c)
		}
	}
	return cards
}

func currentSubmissions(s Session) []Submission {
	var subs []Submission
	for _, sub := range s.Submissions {
		if sub.CardID == s.CurrentCard.ID {
			subs = append(subs, sub)
		}
	}
	return subs
}

func hasVotedThisRound(s Session, v Vote) bool {
	for _, sub := range s.Submissions {
		if sub.ID == v.SubmissionID && sub.CardID == s.CurrentCard.ID {
			return true
		}
	}
	return false
}

func generateCode() string {
	b := make([]byte, 3)
	rand.Read(b)
	return fmt.Sprintf("%06X", b)
}

func generateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
