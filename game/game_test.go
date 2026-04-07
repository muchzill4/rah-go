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

func TestCastVoteAutoAdvancesWhenAllVoted(t *testing.T) {
	s, host, bob, _, bobSub := sessionInVoting()

	s, _ = CastVote(s, host.ID, bobSub.ID)
	if s.Status != Voting {
		t.Fatal("should still be voting after first vote")
	}

	s, _ = CastVote(s, bob.ID, bobSub.ID)
	if s.Status != Discussing {
		t.Fatalf("want auto-advance to Discussing, got %d", s.Status)
	}

	// Winner should be auto-marked
	for _, sub := range s.Submissions {
		if sub.ID == bobSub.ID && !sub.Winner {
			t.Fatal("want winner auto-marked")
		}
	}
}

func TestAdvanceToDiscussingClearWinner(t *testing.T) {
	s, host, _, _, bobSub := sessionInVoting()
	// Only host votes — host force-advances
	s, _ = CastVote(s, host.ID, bobSub.ID)

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

	for _, sub := range s.Submissions {
		if sub.ID == bobSub.ID && !sub.Winner {
			t.Fatal("want winning submission marked as Winner")
		}
	}
}

func TestAdvanceToDiscussingTie(t *testing.T) {
	s, host, bob, hostSub, bobSub := sessionInVoting()
	// Each votes for the other — tie auto-advances
	s, _ = CastVote(s, host.ID, bobSub.ID)
	s, _ = CastVote(s, bob.ID, hostSub.ID)

	if s.Status != Discussing {
		t.Fatalf("want auto-advance to Discussing on tie, got %d", s.Status)
	}

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
	// Tie → auto-advances to discussing
	s, _ = CastVote(s, host.ID, bobSub.ID)
	s, _ = CastVote(s, bob.ID, hostSub.ID)

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
	s, host, bob, hostSub, bobSub := sessionInVoting()
	// Tie → auto-advances to discussing
	s, _ = CastVote(s, host.ID, bobSub.ID)
	s, _ = CastVote(s, bob.ID, hostSub.ID)

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

func TestSubmissionFor(t *testing.T) {
	s, host, bob := sessionInSubmitting()

	if s.SubmissionFor(bob.ID) != nil {
		t.Fatal("want nil before submission")
	}

	s, _ = Submit(s, bob.ID, "bob answer")
	sub := s.SubmissionFor(bob.ID)
	if sub == nil {
		t.Fatal("want submission")
	}
	if sub.Text != "bob answer" {
		t.Fatalf("want text %q, got %q", "bob answer", sub.Text)
	}
	if s.SubmissionFor(host.ID) != nil {
		t.Fatal("want nil for participant who hasn't submitted")
	}
}

func TestHasSubmitted(t *testing.T) {
	s, _, bob := sessionInSubmitting()

	if s.HasSubmitted(bob.ID) {
		t.Fatal("want false before submission")
	}

	s, _ = Submit(s, bob.ID, "answer")
	if !s.HasSubmitted(bob.ID) {
		t.Fatal("want true after submission")
	}
}

func TestSubmittedCount(t *testing.T) {
	s, host, bob := sessionInSubmitting()

	if got := s.SubmittedCount(); got != 0 {
		t.Fatalf("want 0, got %d", got)
	}

	s, _ = Submit(s, bob.ID, "b")
	if got := s.SubmittedCount(); got != 1 {
		t.Fatalf("want 1, got %d", got)
	}

	s, _ = Submit(s, host.ID, "a")
	if got := s.SubmittedCount(); got != 2 {
		t.Fatalf("want 2, got %d", got)
	}
}

func TestCurrentSubmissions(t *testing.T) {
	s, host, bob := sessionInSubmitting()

	if got := s.CurrentSubmissions(); len(got) != 0 {
		t.Fatalf("want 0, got %d", len(got))
	}

	s, _ = Submit(s, host.ID, "a")
	s, _ = Submit(s, bob.ID, "b")
	subs := s.CurrentSubmissions()
	if len(subs) != 2 {
		t.Fatalf("want 2, got %d", len(subs))
	}
}

func TestVotedFor(t *testing.T) {
	s, host, _, _, bobSub := sessionInVoting()

	if s.VotedFor(host.ID) != "" {
		t.Fatal("want empty before voting")
	}

	s, _ = CastVote(s, host.ID, bobSub.ID)
	if got := s.VotedFor(host.ID); got != bobSub.ID {
		t.Fatalf("want %s, got %s", bobSub.ID, got)
	}
}

func TestHasVoted(t *testing.T) {
	s, host, _, _, bobSub := sessionInVoting()

	if s.HasVoted(host.ID) {
		t.Fatal("want false before voting")
	}

	s, _ = CastVote(s, host.ID, bobSub.ID)
	if !s.HasVoted(host.ID) {
		t.Fatal("want true after voting")
	}
}

func TestVotedCount(t *testing.T) {
	s, host, bob, _, bobSub := sessionInVoting()

	if got := s.VotedCount(); got != 0 {
		t.Fatalf("want 0, got %d", got)
	}

	s, _ = CastVote(s, host.ID, bobSub.ID)
	if got := s.VotedCount(); got != 1 {
		t.Fatalf("want 1, got %d", got)
	}

	s, _ = CastVote(s, bob.ID, bobSub.ID)
	if got := s.VotedCount(); got != 2 {
		t.Fatalf("want 2, got %d", got)
	}
}

func TestVoteCount(t *testing.T) {
	s, host, bob, hostSub, bobSub := sessionInVoting()

	if got := s.VoteCount(bobSub.ID); got != 0 {
		t.Fatalf("want 0, got %d", got)
	}

	s, _ = CastVote(s, host.ID, bobSub.ID)
	s, _ = CastVote(s, bob.ID, hostSub.ID)
	if got := s.VoteCount(bobSub.ID); got != 1 {
		t.Fatalf("want 1, got %d", got)
	}
}

func TestParticipantName(t *testing.T) {
	s, host, bob := sessionInSubmitting()

	if got := s.ParticipantName(host.ID); got != "Alice" {
		t.Fatalf("want Alice, got %q", got)
	}
	if got := s.ParticipantName(bob.ID); got != "Bob" {
		t.Fatalf("want Bob, got %q", got)
	}
	if got := s.ParticipantName("nonexistent"); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
}

func TestUndrawnCount(t *testing.T) {
	s, host := testSession()

	if got := s.UndrawnCount(); got != 2 {
		t.Fatalf("want 2, got %d", got)
	}

	s, _ = DrawCard(s, host.ID)
	if got := s.UndrawnCount(); got != 1 {
		t.Fatalf("want 1, got %d", got)
	}
}

func TestDrawnCards(t *testing.T) {
	s, host := testSession()

	if got := s.DrawnCards(); len(got) != 0 {
		t.Fatalf("want 0, got %d", len(got))
	}

	s, _ = DrawCard(s, host.ID)
	drawn := s.DrawnCards()
	if len(drawn) != 1 {
		t.Fatalf("want 1, got %d", len(drawn))
	}
	if drawn[0].ID != s.CurrentCard.ID {
		t.Fatalf("want %s, got %s", s.CurrentCard.ID, drawn[0].ID)
	}
}

func TestWinnerForCard(t *testing.T) {
	s, host, bob, _, bobSub := sessionInVoting()
	s, _ = CastVote(s, host.ID, bobSub.ID)
	s, _ = CastVote(s, bob.ID, bobSub.ID)

	w := s.WinnerForCard(s.CurrentCard.ID)
	if w == nil {
		t.Fatal("want winner")
	}
	if w.ID != bobSub.ID {
		t.Fatalf("want %s, got %s", bobSub.ID, w.ID)
	}
	if s.WinnerForCard("nonexistent") != nil {
		t.Fatal("want nil for unknown card")
	}
}

func TestHasSubmissionsForCard(t *testing.T) {
	s, _, bob := sessionInSubmitting()
	cardID := s.CurrentCard.ID

	if s.HasSubmissionsForCard(cardID) {
		t.Fatal("want false before submissions")
	}

	s, _ = Submit(s, bob.ID, "answer")
	if !s.HasSubmissionsForCard(cardID) {
		t.Fatal("want true after submission")
	}
}

func TestWinnerAndTiedSubmissions(t *testing.T) {
	s, host, bob, hostSub, bobSub := sessionInVoting()

	// No votes — no winner, no ties
	if s.Winner() != nil {
		t.Fatal("want nil winner with no votes")
	}
	if len(s.TiedSubmissions()) != 0 {
		t.Fatal("want no tied submissions with no votes")
	}

	// Clear winner
	s, _ = CastVote(s, host.ID, bobSub.ID)
	s, _ = CastVote(s, bob.ID, bobSub.ID)
	if w := s.Winner(); w == nil || w.ID != bobSub.ID {
		t.Fatalf("want winner %s", bobSub.ID)
	}
	if len(s.TiedSubmissions()) != 0 {
		t.Fatal("want no tied submissions with clear winner")
	}

	// Tie — reset votes
	s2, _, _, _, _ := sessionInVoting()
	s2, _ = CastVote(s2, host.ID, bobSub.ID)
	s2, _ = CastVote(s2, bob.ID, hostSub.ID)
	if s2.Winner() != nil {
		t.Fatal("want nil winner on tie")
	}
	if len(s2.TiedSubmissions()) != 2 {
		t.Fatalf("want 2 tied, got %d", len(s2.TiedSubmissions()))
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
