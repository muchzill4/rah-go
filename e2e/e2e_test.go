package e2e

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/playwright-community/playwright-go"
)

var (
	baseURL string
	pw      *playwright.Playwright
)

func TestMain(m *testing.M) {
	if err := playwright.Install(&playwright.RunOptions{Browsers: []string{"chromium"}}); err != nil {
		fmt.Fprintf(os.Stderr, "playwright install failed: %v\n", err)
		os.Exit(1)
	}

	var err error
	pw, err = playwright.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "playwright start failed: %v\n", err)
		os.Exit(1)
	}

	bin, err := buildBinary()
	if err != nil {
		pw.Stop()
		fmt.Fprintf(os.Stderr, "build failed: %v\n", err)
		os.Exit(1)
	}

	port, err := freePort()
	if err != nil {
		os.Remove(bin)
		pw.Stop()
		fmt.Fprintf(os.Stderr, "no free port: %v\n", err)
		os.Exit(1)
	}

	baseURL = fmt.Sprintf("http://localhost:%d", port)

	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), fmt.Sprintf("PORT=%d", port))
	if err := cmd.Start(); err != nil {
		os.Remove(bin)
		pw.Stop()
		fmt.Fprintf(os.Stderr, "start failed: %v\n", err)
		os.Exit(1)
	}

	if err := waitForReady(baseURL, 5*time.Second); err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		os.Remove(bin)
		pw.Stop()
		fmt.Fprintf(os.Stderr, "server not ready: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	cmd.Process.Kill()
	cmd.Wait()
	os.Remove(bin)
	pw.Stop()

	os.Exit(code)
}

func buildBinary() (string, error) {
	f, err := os.CreateTemp("", "rah-go-test-*")
	if err != nil {
		return "", err
	}
	bin := f.Name()
	f.Close()

	cmd := exec.Command("go", "build", "-o", bin, "..")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.Remove(bin)
		return "", err
	}
	return bin, nil
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port, nil
}

func waitForReady(base string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/")
		if err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("server did not start within %v", timeout)
}

// player simulates a browser session with its own cookie jar.
type player struct {
	t      *testing.T
	client *http.Client
	base   string
}

func newPlayer(t *testing.T) *player {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &player{
		t:      t,
		client: &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		base:   baseURL,
	}
}

func (p *player) get(path string) string {
	p.t.Helper()
	resp, err := p.client.Get(p.base + path)
	if err != nil {
		p.t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body)
}

func (p *player) post(path string, form url.Values) string {
	p.t.Helper()
	resp, err := p.client.PostForm(p.base+path, form)
	if err != nil {
		p.t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusSeeOther {
		loc := resp.Header.Get("Location")
		return p.get(loc)
	}
	if resp.StatusCode >= 400 {
		p.t.Fatalf("POST %s returned %d: %s", path, resp.StatusCode, string(body))
	}
	return string(body)
}

func (p *player) postOK(path string, form url.Values) {
	p.t.Helper()
	resp, err := p.client.PostForm(p.base+path, form)
	if err != nil {
		p.t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		p.t.Fatalf("POST %s returned %d: %s", path, resp.StatusCode, string(body))
	}
}

func (p *player) postExpectError(path string, form url.Values) string {
	p.t.Helper()
	resp, err := p.client.PostForm(p.base+path, form)
	if err != nil {
		p.t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 400 {
		p.t.Fatalf("POST %s expected error, got %d", path, resp.StatusCode)
	}
	return string(body)
}

func (p *player) postStatus(path string, form url.Values) int {
	p.t.Helper()
	resp, err := p.client.PostForm(p.base+path, form)
	if err != nil {
		p.t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func (p *player) viewSession(code string) string {
	p.t.Helper()
	return p.get("/sessions/" + code)
}

func (p *player) createSession(name string, cards []string) string {
	p.t.Helper()
	body := p.post("/sessions", url.Values{
		"host_name": {name},
		"timer":     {"60"},
		"cards":     {strings.Join(cards, "\n")},
	})
	return extractCode(p.t, body)
}

func (p *player) joinSession(code, name string) {
	p.t.Helper()
	p.post("/sessions/"+code+"/participants", url.Values{"name": {name}})
}

func (p *player) draw(code string)                         { p.t.Helper(); p.postOK("/sessions/"+code+"/draw", nil) }
func (p *player) submit(code, text string)                 { p.t.Helper(); p.postOK("/sessions/"+code+"/submissions", url.Values{"text": {text}}) }
func (p *player) advance(code string)                      { p.t.Helper(); p.postOK("/sessions/"+code+"/advance", nil) }
func (p *player) vote(code, submissionID string)           { p.t.Helper(); p.postOK("/sessions/"+code+"/votes", url.Values{"submission_id": {submissionID}}) }
func (p *player) pickWinner(code, submissionID string)     { p.t.Helper(); p.postOK("/sessions/"+code+"/winners", url.Values{"submission_id": {submissionID}}) }
func (p *player) skip(code string)                         { p.t.Helper(); p.postOK("/sessions/"+code+"/skip", nil) }
func (p *player) finish(code string)                       { p.t.Helper(); p.postOK("/sessions/"+code+"/summary", nil) }

var codeRe = regexp.MustCompile(`class="session__code"[^>]*>(?:[^<]*<strong>)([A-F0-9]{6})</strong>`)

func extractCode(t *testing.T, body string) string {
	t.Helper()
	m := codeRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatal("could not find session code in response")
	}
	return m[1]
}

func extractSubmissionIDs(body string) []string {
	re := regexp.MustCompile(`"submission_id":"([^"]+)"`)
	matches := re.FindAllStringSubmatch(body, -1)
	var ids []string
	for _, m := range matches {
		ids = append(ids, m[1])
	}
	return ids
}

func assertContains(t *testing.T, body, substr string) {
	t.Helper()
	if !strings.Contains(body, substr) {
		t.Errorf("expected body to contain %q", substr)
	}
}

func assertNotContains(t *testing.T, body, substr string) {
	t.Helper()
	if strings.Contains(body, substr) {
		t.Errorf("expected body NOT to contain %q", substr)
	}
}

var testCards = []string{
	"The worst part of our sprint was {blank}.",
	"Next retro we should discuss {blank}.",
}

func TestThreePlayersCompleteRoundWithClearWinner(t *testing.T) {
	alice := newPlayer(t)
	bob := newPlayer(t)
	carol := newPlayer(t)

	code := alice.createSession("Alice", testCards)
	bob.joinSession(code, "Bob")
	carol.joinSession(code, "Carol")

	// Lobby shows all participants
	body := alice.viewSession(code)
	assertContains(t, body, "Alice")
	assertContains(t, body, "Bob")
	assertContains(t, body, "Carol")
	assertContains(t, body, "Waiting for everyone to join")

	// Non-host sees waiting message
	body = bob.viewSession(code)
	assertContains(t, body, "Waiting for the host to start")

	// Draw first card
	alice.draw(code)

	body = alice.viewSession(code)
	assertContains(t, body, "blank-input")
	assertContains(t, body, "1 cards remaining")

	// Everyone submits
	alice.submit(code, "Too many standups")
	bob.submit(code, "Insufficient coffee")
	carol.submit(code, "The retrospective itself")

	// Advance to voting
	alice.advance(code)

	body = alice.viewSession(code)
	subIDs := extractSubmissionIDs(body)
	if len(subIDs) < 3 {
		t.Fatalf("expected at least 3 submission IDs, got %d", len(subIDs))
	}

	// Alice and Carol vote for Bob's answer (2 votes), Bob votes for Carol's
	alice.vote(code, subIDs[1])
	carol.vote(code, subIDs[1])
	bob.vote(code, subIDs[2])

	// All voted -> auto-advanced to discussing
	body = alice.viewSession(code)
	assertContains(t, body, "Insufficient coffee")
	assertContains(t, body, "Bob")
	assertContains(t, body, "&#x1f3c6;") // trophy

	body = bob.viewSession(code)
	assertContains(t, body, "Insufficient coffee")

	body = carol.viewSession(code)
	assertContains(t, body, "Insufficient coffee")
}

func TestHostBreaksTie(t *testing.T) {
	alice := newPlayer(t)
	bob := newPlayer(t)

	code := alice.createSession("Alice", testCards)
	bob.joinSession(code, "Bob")

	alice.draw(code)
	alice.submit(code, "Too many standups")
	bob.submit(code, "Not enough doughnuts")

	alice.advance(code) // to voting

	body := alice.viewSession(code)
	subIDs := extractSubmissionIDs(body)

	// Each votes for the other — tie
	alice.vote(code, subIDs[1])
	bob.vote(code, subIDs[0])

	// All voted -> auto-advanced to discussing (tie)
	body = alice.viewSession(code)
	assertContains(t, body, "tie")
	assertContains(t, body, "pick the winner")

	// Bob sees tie but no pick button
	body = bob.viewSession(code)
	assertContains(t, body, "tie")
	assertContains(t, body, "Waiting for the host to pick the winner")
	assertNotContains(t, body, "Pick this one")

	// Host picks Alice's answer
	aliceBody := alice.viewSession(code)
	aliceTiedIDs := extractSubmissionIDs(aliceBody)
	alice.pickWinner(code, aliceTiedIDs[0])

	// After picking, host can draw the next card
	alice.draw(code)

	body = alice.viewSession(code)
	assertContains(t, body, "blank-input")
}

func TestAutoAdvanceWhenAllVoted(t *testing.T) {
	alice := newPlayer(t)
	bob := newPlayer(t)

	code := alice.createSession("Alice", testCards)
	bob.joinSession(code, "Bob")

	alice.draw(code)
	alice.submit(code, "Answer A")
	bob.submit(code, "Answer B")

	alice.advance(code) // to voting

	body := alice.viewSession(code)
	subIDs := extractSubmissionIDs(body)

	// Both vote for the same answer -> auto-advances to discussing
	alice.vote(code, subIDs[0])
	bob.vote(code, subIDs[0])

	body = alice.viewSession(code)
	assertContains(t, body, "Discussion time")
	assertContains(t, body, "&#x1f3c6;") // trophy
}

func TestCannotSubmitTwice(t *testing.T) {
	alice := newPlayer(t)
	bob := newPlayer(t)

	code := alice.createSession("Alice", testCards)
	bob.joinSession(code, "Bob")

	alice.draw(code)
	alice.submit(code, "First answer")

	errBody := alice.postExpectError("/sessions/"+code+"/submissions", url.Values{"text": {"Second answer"}})
	assertContains(t, errBody, "already submitted")

	body := alice.viewSession(code)
	assertContains(t, body, "Submitted")
}

func TestHostCanSkipCard(t *testing.T) {
	alice := newPlayer(t)
	bob := newPlayer(t)

	code := alice.createSession("Alice", testCards)
	bob.joinSession(code, "Bob")

	alice.draw(code)

	body := alice.viewSession(code)
	assertContains(t, body, "blank-input")

	alice.skip(code)

	// Now in discussing phase with no winner
	body = alice.viewSession(code)
	assertContains(t, body, "No votes were cast")

	// Draw next card and complete a round
	alice.draw(code)
	alice.submit(code, "Too many standups")
	bob.submit(code, "Not enough coffee")

	alice.advance(code) // to voting

	body = alice.viewSession(code)
	subIDs := extractSubmissionIDs(body)
	alice.vote(code, subIDs[1])
	bob.vote(code, subIDs[1])

	body = alice.viewSession(code)
	assertContains(t, body, "Not enough coffee")
	assertContains(t, body, "&#x1f3c6;") // trophy
}

func TestHostCanEndSessionEarly(t *testing.T) {
	alice := newPlayer(t)
	bob := newPlayer(t)

	code := alice.createSession("Alice", testCards)
	bob.joinSession(code, "Bob")

	// Complete one round
	alice.draw(code)
	alice.submit(code, "Forgetting the action items")
	bob.submit(code, "Blaming the process")

	alice.advance(code) // to voting

	body := alice.viewSession(code)
	subIDs := extractSubmissionIDs(body)
	alice.vote(code, subIDs[1])
	bob.vote(code, subIDs[1])

	// Auto-advanced to discussing, now finish
	alice.finish(code)

	body = alice.viewSession(code)
	assertContains(t, body, "Session summary")
	assertContains(t, body, "Blaming the process")
}

func TestSessionFinishesWhenAllCardsDrawn(t *testing.T) {
	alice := newPlayer(t)
	bob := newPlayer(t)

	// Only 1 card
	code := alice.createSession("Alice", []string{"The worst part was {blank}."})
	bob.joinSession(code, "Bob")

	body := alice.viewSession(code)
	assertContains(t, body, "1 cards remaining")

	alice.draw(code)
	alice.submit(code, "Everything")
	bob.submit(code, "Nothing")

	alice.advance(code) // to voting

	body = alice.viewSession(code)
	subIDs := extractSubmissionIDs(body)
	alice.vote(code, subIDs[0])
	bob.vote(code, subIDs[0])

	// Auto-advanced to discussing, no more cards -> draw triggers finish
	alice.draw(code)

	body = alice.viewSession(code)
	assertContains(t, body, "Session summary")
}

func TestPlayersSeeLobbyParticipants(t *testing.T) {
	alice := newPlayer(t)
	bob := newPlayer(t)
	carol := newPlayer(t)

	code := alice.createSession("Alice", testCards)
	bob.joinSession(code, "Bob")
	carol.joinSession(code, "Carol")

	for _, p := range []*player{alice, bob, carol} {
		body := p.viewSession(code)
		assertContains(t, body, "Alice")
		assertContains(t, body, "Bob")
		assertContains(t, body, "Carol")
	}
}

func TestCreateSessionRequiresName(t *testing.T) {
	p := newPlayer(t)

	errBody := p.postExpectError("/sessions", url.Values{
		"host_name": {""},
		"timer":     {"60"},
		"cards":     {"Something {blank}."},
	})
	assertContains(t, errBody, "name is required")
}

func TestCreateSessionRequiresCardsWithBlanks(t *testing.T) {
	p := newPlayer(t)

	errBody := p.postExpectError("/sessions", url.Values{
		"host_name": {"Alice"},
		"timer":     {"60"},
		"cards":     {"No blank here\nAlso missing a blank"},
	})
	assertContains(t, errBody, "at least one card with {blank} is required")
}

func TestCreateSessionAcceptsUnderscoreShorthand(t *testing.T) {
	p := newPlayer(t)

	code := p.createSession("Alice", []string{"Our team is clearly _."})
	body := p.viewSession(code)
	assertContains(t, body, code)
}

func TestJoinRequiresName(t *testing.T) {
	alice := newPlayer(t)
	bob := newPlayer(t)

	code := alice.createSession("Alice", testCards)
	errBody := bob.postExpectError("/sessions/"+code+"/participants", url.Values{"name": {""}})
	assertContains(t, errBody, "name is required")
}

func TestJoinOnlyInLobby(t *testing.T) {
	alice := newPlayer(t)
	bob := newPlayer(t)

	code := alice.createSession("Alice", testCards)
	alice.draw(code)

	errBody := bob.postExpectError("/sessions/"+code+"/participants", url.Values{"name": {"Bob"}})
	assertContains(t, errBody, "wrong session status")
}

func TestNonHostCannotDraw(t *testing.T) {
	alice := newPlayer(t)
	bob := newPlayer(t)

	code := alice.createSession("Alice", testCards)
	bob.joinSession(code, "Bob")

	status := bob.postStatus("/sessions/"+code+"/draw", nil)
	if status != http.StatusForbidden {
		t.Errorf("expected 403, got %d", status)
	}
}

func TestSkipShowsAsSkippedInSummary(t *testing.T) {
	alice := newPlayer(t)
	bob := newPlayer(t)

	code := alice.createSession("Alice", testCards)
	bob.joinSession(code, "Bob")

	// Draw and skip first card
	alice.draw(code)
	alice.skip(code)

	// Draw second card and complete
	alice.draw(code)
	alice.submit(code, "Something good")
	bob.submit(code, "Something better")

	alice.advance(code) // to voting
	body := alice.viewSession(code)
	subIDs := extractSubmissionIDs(body)
	alice.vote(code, subIDs[0])
	bob.vote(code, subIDs[0])

	// Finish
	alice.finish(code)

	body = alice.viewSession(code)
	assertContains(t, body, "Session summary")
	assertContains(t, body, "Skipped")
	assertContains(t, body, "Something good")
}

func TestUnjoinedVisitorSeesJoinForm(t *testing.T) {
	alice := newPlayer(t)
	visitor := newPlayer(t)

	code := alice.createSession("Alice", testCards)

	body := visitor.viewSession(code)
	assertContains(t, body, "Join this session")
	assertContains(t, body, "Your name")
}

func TestNonexistentSessionReturns404(t *testing.T) {
	p := newPlayer(t)

	resp, err := p.client.Get(p.base + "/sessions/ZZZZZZ")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}
