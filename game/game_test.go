package game

import (
	"errors"
	"testing"
)

func TestNewSession(t *testing.T) {
	cards := []string{
		"The best thing about {blank} is {blank}.",
		"Nobody expects {blank}.",
	}
	s, host := NewSession("Alice", 60, cards)

	if s.Status != Lobby {
		t.Fatalf("want status Lobby, got %d", s.Status)
	}
	if len(s.Code) != 6 {
		t.Fatalf("want 6-char code, got %q", s.Code)
	}
	if s.SubmissionTimerSeconds != 60 {
		t.Fatalf("want timer 60, got %d", s.SubmissionTimerSeconds)
	}
	if len(s.Cards) != 2 {
		t.Fatalf("want 2 cards, got %d", len(s.Cards))
	}
	if s.Cards[0].Text != cards[0] {
		t.Fatalf("want card text %q, got %q", cards[0], s.Cards[0].Text)
	}
	if len(s.Participants) != 1 {
		t.Fatalf("want 1 participant, got %d", len(s.Participants))
	}
	if !host.Host {
		t.Fatal("want host=true")
	}
	if host.Name != "Alice" {
		t.Fatalf("want name Alice, got %q", host.Name)
	}
	if len(host.Token) != 32 {
		t.Fatalf("want 32-char token, got %d chars", len(host.Token))
	}
}

func testSession() (Session, Participant) {
	return NewSession("Alice", 60, []string{
		"The best thing about {blank} is {blank}.",
		"Nobody expects {blank}.",
	})
}

func TestJoin(t *testing.T) {
	s, _ := testSession()

	s, bob, err := Join(s, "Bob")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.Participants) != 2 {
		t.Fatalf("want 2 participants, got %d", len(s.Participants))
	}
	if bob.Host {
		t.Fatal("second participant should not be host")
	}
	if bob.Name != "Bob" {
		t.Fatalf("want name Bob, got %q", bob.Name)
	}
}

func TestJoinRequiresLobby(t *testing.T) {
	s, _ := testSession()
	s.Status = Submitting

	_, _, err := Join(s, "Bob")
	if !errors.Is(err, ErrWrongStatus) {
		t.Fatalf("want ErrWrongStatus, got %v", err)
	}
}

func TestDrawCard(t *testing.T) {
	s, host := testSession()

	s, err := DrawCard(s, host.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Status != Submitting {
		t.Fatalf("want status Submitting, got %d", s.Status)
	}
	if s.CurrentCard == nil {
		t.Fatal("want current card set")
	}
	if !s.DrawnCardIDs[s.CurrentCard.ID] {
		t.Fatal("drawn card should be tracked")
	}
	if s.SubmissionDeadline.IsZero() {
		t.Fatal("want submission deadline set")
	}
}

func TestDrawCardRequiresHost(t *testing.T) {
	s, _ := testSession()
	s, bob, _ := Join(s, "Bob")

	_, err := DrawCard(s, bob.ID)
	if !errors.Is(err, ErrNotHost) {
		t.Fatalf("want ErrNotHost, got %v", err)
	}
}

func TestDrawCardNoDuplicates(t *testing.T) {
	s, host := testSession() // 2 cards

	s, _ = DrawCard(s, host.ID)
	firstCard := s.CurrentCard.ID

	// Advance through submitting→voting→discussing to draw again
	s.Status = Lobby

	s, _ = DrawCard(s, host.ID)
	if s.CurrentCard.ID == firstCard {
		t.Fatal("should not draw the same card twice")
	}
}

func TestDrawCardAllDrawnFinishes(t *testing.T) {
	s, host := testSession()
	for _, c := range s.Cards {
		s.DrawnCardIDs[c.ID] = true
	}

	s, err := DrawCard(s, host.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Status != Finished {
		t.Fatalf("want status Finished when no cards left, got %d", s.Status)
	}
}

func sessionInSubmitting() (Session, Participant, Participant) {
	s, host := testSession()
	s, bob, _ := Join(s, "Bob")
	s, _ = DrawCard(s, host.ID)
	return s, host, bob
}

func TestSubmit(t *testing.T) {
	s, _, bob := sessionInSubmitting()

	s, err := Submit(s, bob.ID, "my answer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.Submissions) != 1 {
		t.Fatalf("want 1 submission, got %d", len(s.Submissions))
	}
	sub := s.Submissions[0]
	if sub.ParticipantID != bob.ID {
		t.Fatalf("want participant %s, got %s", bob.ID, sub.ParticipantID)
	}
	if sub.CardID != s.CurrentCard.ID {
		t.Fatalf("want card %s, got %s", s.CurrentCard.ID, sub.CardID)
	}
	if sub.Text != "my answer" {
		t.Fatalf("want text %q, got %q", "my answer", sub.Text)
	}
}

func TestSubmitRequiresSubmittingStatus(t *testing.T) {
	s, _ := testSession()

	_, err := Submit(s, "p-1", "answer")
	if !errors.Is(err, ErrWrongStatus) {
		t.Fatalf("want ErrWrongStatus, got %v", err)
	}
}

func TestSubmitOncePerParticipantPerCard(t *testing.T) {
	s, _, bob := sessionInSubmitting()

	s, _ = Submit(s, bob.ID, "first")
	_, err := Submit(s, bob.ID, "second")
	if !errors.Is(err, ErrAlreadySubmitted) {
		t.Fatalf("want ErrAlreadySubmitted, got %v", err)
	}
}

func TestAdvanceToVoting(t *testing.T) {
	s, host, bob := sessionInSubmitting()
	s, _ = Submit(s, bob.ID, "answer")

	s, err := AdvanceToVoting(s, host.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Status != Voting {
		t.Fatalf("want status Voting, got %d", s.Status)
	}
}

func TestAdvanceToVotingRequiresHost(t *testing.T) {
	s, _, bob := sessionInSubmitting()

	_, err := AdvanceToVoting(s, bob.ID)
	if !errors.Is(err, ErrNotHost) {
		t.Fatalf("want ErrNotHost, got %v", err)
	}
}

func TestAdvanceToVotingRequiresSubmitting(t *testing.T) {
	s, host := testSession()

	_, err := AdvanceToVoting(s, host.ID)
	if !errors.Is(err, ErrWrongStatus) {
		t.Fatalf("want ErrWrongStatus, got %v", err)
	}
}

func sessionInVoting() (Session, Participant, Participant, Submission, Submission) {
	s, host, bob := sessionInSubmitting()
	s, _ = Submit(s, host.ID, "host answer")
	s, _ = Submit(s, bob.ID, "bob answer")
	s, _ = AdvanceToVoting(s, host.ID)
	hostSub := s.Submissions[0]
	bobSub := s.Submissions[1]
	return s, host, bob, hostSub, bobSub
}

func TestVote(t *testing.T) {
	s, _, bob, _, bobSub := sessionInVoting()

	s, err := CastVote(s, bob.ID, bobSub.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.Votes) != 1 {
		t.Fatalf("want 1 vote, got %d", len(s.Votes))
	}
	if s.Votes[0].SubmissionID != bobSub.ID {
		t.Fatalf("want vote for %s, got %s", bobSub.ID, s.Votes[0].SubmissionID)
	}
}

func TestVoteRequiresVotingStatus(t *testing.T) {
	s, _, bob := sessionInSubmitting()

	_, err := CastVote(s, bob.ID, "sub-1")
	if !errors.Is(err, ErrWrongStatus) {
		t.Fatalf("want ErrWrongStatus, got %v", err)
	}
}

func TestVoteOncePerParticipantPerRound(t *testing.T) {
	s, _, bob, hostSub, bobSub := sessionInVoting()

	s, _ = CastVote(s, bob.ID, hostSub.ID)
	_, err := CastVote(s, bob.ID, bobSub.ID)
	if !errors.Is(err, ErrAlreadyVoted) {
		t.Fatalf("want ErrAlreadyVoted, got %v", err)
	}
}

func TestAdvanceToDiscussingClearWinner(t *testing.T) {
	s, host, bob, _, bobSub := sessionInVoting()
	// Both vote for bob's submission
	s, _ = CastVote(s, host.ID, bobSub.ID)
	s, _ = CastVote(s, bob.ID, bobSub.ID)

	s, err := AdvanceToDiscussing(s, host.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Status != Discussing {
		t.Fatalf("want status Discussing, got %d", s.Status)
	}

	winner, tied := WinningSubmission(s)
	if winner == nil {
		t.Fatal("want a winner")
	}
	if winner.ID != bobSub.ID {
		t.Fatalf("want winner %s, got %s", bobSub.ID, winner.ID)
	}
	if len(tied) != 0 {
		t.Fatalf("want no tie, got %d tied", len(tied))
	}
}

func TestAdvanceToDiscussingTie(t *testing.T) {
	s, host, bob, hostSub, bobSub := sessionInVoting()
	// Each votes for the other's submission
	s, _ = CastVote(s, host.ID, bobSub.ID)
	s, _ = CastVote(s, bob.ID, hostSub.ID)

	s, _ = AdvanceToDiscussing(s, host.ID)

	winner, tied := WinningSubmission(s)
	if winner != nil {
		t.Fatal("want no clear winner on tie")
	}
	if len(tied) != 2 {
		t.Fatalf("want 2 tied submissions, got %d", len(tied))
	}
}

func TestAdvanceToDiscussingNoVotes(t *testing.T) {
	s, host, _, _, _ := sessionInVoting()

	s, _ = AdvanceToDiscussing(s, host.ID)

	winner, tied := WinningSubmission(s)
	if winner != nil {
		t.Fatal("want no winner when no votes")
	}
	if len(tied) != 0 {
		t.Fatalf("want no tied submissions, got %d", len(tied))
	}
}

func TestPickWinner(t *testing.T) {
	s, host, bob, hostSub, bobSub := sessionInVoting()
	s, _ = CastVote(s, host.ID, bobSub.ID)
	s, _ = CastVote(s, bob.ID, hostSub.ID)
	s, _ = AdvanceToDiscussing(s, host.ID)

	s, err := PickWinner(s, host.ID, bobSub.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var won bool
	for _, sub := range s.Submissions {
		if sub.ID == bobSub.ID {
			won = sub.Winner
		}
	}
	if !won {
		t.Fatal("want submission marked as winner")
	}
}

func TestPickWinnerRequiresHost(t *testing.T) {
	s, host, bob, _, bobSub := sessionInVoting()
	s, _ = CastVote(s, host.ID, bobSub.ID)
	s, _ = CastVote(s, bob.ID, bobSub.ID)
	s, _ = AdvanceToDiscussing(s, host.ID)

	_, err := PickWinner(s, bob.ID, bobSub.ID)
	if !errors.Is(err, ErrNotHost) {
		t.Fatalf("want ErrNotHost, got %v", err)
	}
}

func TestSkipCard(t *testing.T) {
	s, host, _ := sessionInSubmitting()

	s, err := SkipCard(s, host.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Status != Discussing {
		t.Fatalf("want status Discussing, got %d", s.Status)
	}
}

func TestSkipCardRequiresHost(t *testing.T) {
	s, _, bob := sessionInSubmitting()

	_, err := SkipCard(s, bob.ID)
	if !errors.Is(err, ErrNotHost) {
		t.Fatalf("want ErrNotHost, got %v", err)
	}
}

func TestFinish(t *testing.T) {
	s, host, _ := sessionInSubmitting()

	s, err := Finish(s, host.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Status != Finished {
		t.Fatalf("want status Finished, got %d", s.Status)
	}
}

func TestFinishRequiresHost(t *testing.T) {
	s, _, bob := sessionInSubmitting()

	_, err := Finish(s, bob.ID)
	if !errors.Is(err, ErrNotHost) {
		t.Fatalf("want ErrNotHost, got %v", err)
	}
}

func TestAllSubmitted(t *testing.T) {
	s, host, bob := sessionInSubmitting()

	if AllSubmitted(s) {
		t.Fatal("want false when no submissions")
	}

	s, _ = Submit(s, host.ID, "a")
	if AllSubmitted(s) {
		t.Fatal("want false when only one submitted")
	}

	s, _ = Submit(s, bob.ID, "b")
	if !AllSubmitted(s) {
		t.Fatal("want true when all submitted")
	}
}

func TestAllVoted(t *testing.T) {
	s, host, bob, _, bobSub := sessionInVoting()

	if AllVoted(s) {
		t.Fatal("want false when no votes")
	}

	s, _ = CastVote(s, host.ID, bobSub.ID)
	if AllVoted(s) {
		t.Fatal("want false when only one voted")
	}

	s, _ = CastVote(s, bob.ID, bobSub.ID)
	if !AllVoted(s) {
		t.Fatal("want true when all voted")
	}
}
