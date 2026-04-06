package e2e

import (
	"fmt"
	"strings"
	"testing"

	"github.com/playwright-community/playwright-go"
)

// sseTimeout is how long to wait for SSE-driven DOM updates.
// Generous because the first page load fetches htmx from CDN, delaying SSE connection.
const sseTimeout = 15_000

func newBrowserPage(t *testing.T) playwright.Page {
	t.Helper()
	browser, err := pw.Chromium.Launch()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { browser.Close() })

	page, err := browser.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	return page
}

// expect wraps a playwright assertion error, failing the test immediately if non-nil.
func expect(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func createSessionViaForm(t *testing.T, page playwright.Page, name string, cards []string) string {
	t.Helper()
	_, err := page.Goto(baseURL)
	if err != nil {
		t.Fatal(err)
	}

	page.Locator("#host_name").Fill(name)
	page.Locator("#cards").Fill(strings.Join(cards, "\n"))
	page.Locator("button:has-text('Create session')").Click()

	// htmx intercepts the form POST and follows the HX-Redirect
	page.WaitForURL("**/sessions/*")

	code, err := page.Locator(".session__code strong").TextContent()
	if err != nil {
		t.Fatal(err)
	}
	return code
}

func joinSessionViaForm(t *testing.T, page playwright.Page, code, name string) {
	t.Helper()
	_, err := page.Goto(baseURL + "/sessions/" + code)
	if err != nil {
		t.Fatal(err)
	}

	page.Locator("#name").Fill(name)
	page.Locator("button:has-text('Join')").Click()

	// Form submits and redirects back to the session page
	page.Locator(".participant-list__item").First().WaitFor()
}

func fillBlank(page playwright.Page, text string) {
	blank := page.Locator(".blank-input").First()
	blank.Click()
	blank.PressSequentially(text)
}

func TestBrowserCreateSessionShowsLobby(t *testing.T) {
	page := newBrowserPage(t)
	pw := playwright.NewPlaywrightAssertions()

	code := createSessionViaForm(t, page, "Alice", testCards)

	expect(t, pw.Locator(page.Locator(".session__code strong")).ToHaveText(code))
	expect(t, pw.Locator(page.Locator(".participant-list__item")).ToHaveCount(1))
	expect(t, pw.Page(page).ToHaveURL(fmt.Sprintf("%s/sessions/%s", baseURL, code)))
}

func TestBrowserCreateSessionValidation(t *testing.T) {
	page := newBrowserPage(t)
	pw := playwright.NewPlaywrightAssertions()

	_, err := page.Goto(baseURL)
	if err != nil {
		t.Fatal(err)
	}

	// Fill name but provide cards without {blank} — tests server-side card validation
	page.Locator("#host_name").Fill("Alice")
	page.Locator("#cards").Fill("No blank here")
	page.Locator("button:has-text('Create session')").Click()

	// htmx should swap error into #create-errors
	expect(t, pw.Locator(page.Locator("#create-errors")).ToContainText("{blank}"))
}

func TestBrowserJoinAndLobbyUpdatesViaSSE(t *testing.T) {
	alice := newBrowserPage(t)
	bob := newBrowserPage(t)
	pw := playwright.NewPlaywrightAssertions(sseTimeout)

	code := createSessionViaForm(t, alice, "Alice", testCards)

	// Alice sees herself in the lobby (inside the SSE-swapped game area)
	expect(t, pw.Locator(alice.Locator(".participant-list__item")).ToHaveCount(1))

	// Bob joins — Alice's lobby should update via SSE
	joinSessionViaForm(t, bob, code, "Bob")

	expect(t, pw.Locator(alice.Locator(".participant-list__item")).ToHaveCount(2))
	expect(t, pw.Locator(alice.Locator(".participant-list__name:has-text('Bob')")).ToBeVisible())
}

func TestBrowserFullGameFlow(t *testing.T) {
	alice := newBrowserPage(t)
	bob := newBrowserPage(t)
	pw := playwright.NewPlaywrightAssertions(sseTimeout)

	code := createSessionViaForm(t, alice, "Alice", testCards)
	joinSessionViaForm(t, bob, code, "Bob")

	// Alice draws first card
	alice.Locator("button:has-text('Draw first card')").Click()

	// Both should see the submission form via SSE
	expect(t, pw.Locator(alice.Locator(".blank-input")).ToBeVisible())
	expect(t, pw.Locator(bob.Locator(".blank-input")).ToBeVisible())

	// Both submit answers using the contenteditable blank
	fillBlank(alice, "Too many standups")
	alice.Locator("button:has-text('Submit')").Click()
	expect(t, pw.Locator(alice.Locator("text=Submitted")).ToBeVisible())

	fillBlank(bob, "Insufficient coffee")
	bob.Locator("button:has-text('Submit')").Click()
	expect(t, pw.Locator(bob.Locator("text=Submitted")).ToBeVisible())

	// Alice advances to voting
	alice.Locator("button:has-text('Close submissions')").Click()

	// Both should see voting UI via SSE
	expect(t, pw.Locator(alice.Locator("text=Time to vote")).ToBeVisible())
	expect(t, pw.Locator(bob.Locator("text=Time to vote")).ToBeVisible())

	// Both vote for the first option
	alice.Locator("button:has-text('Vote')").First().Click()
	bob.Locator("button:has-text('Vote')").First().Click()

	// Auto-advance to discussing — both see the winner via SSE
	expect(t, pw.Locator(alice.Locator("text=Discussion time")).ToBeVisible())
	expect(t, pw.Locator(bob.Locator("text=Discussion time")).ToBeVisible())
	expect(t, pw.Locator(alice.Locator(".prompt-card__author:has-text('🏆')")).ToBeVisible())
}

func TestBrowserHostCanEndSession(t *testing.T) {
	alice := newBrowserPage(t)
	bob := newBrowserPage(t)
	pw := playwright.NewPlaywrightAssertions(sseTimeout)

	cards := []string{"The worst part was {blank}."}
	code := createSessionViaForm(t, alice, "Alice", cards)
	joinSessionViaForm(t, bob, code, "Bob")

	// Play through one round
	alice.Locator("button:has-text('Draw first card')").Click()
	expect(t, pw.Locator(alice.Locator(".blank-input")).ToBeVisible())

	fillBlank(alice, "Everything")
	alice.Locator("button:has-text('Submit')").Click()
	expect(t, pw.Locator(alice.Locator("text=Submitted")).ToBeVisible())

	fillBlank(bob, "Nothing")
	bob.Locator("button:has-text('Submit')").Click()
	expect(t, pw.Locator(bob.Locator("text=Submitted")).ToBeVisible())

	alice.Locator("button:has-text('Close submissions')").Click()

	// Both should see voting UI via SSE
	expect(t, pw.Locator(alice.Locator("text=Time to vote")).ToBeVisible())
	expect(t, pw.Locator(bob.Locator("text=Time to vote")).ToBeVisible())

	// Both vote for the first option
	alice.Locator("button:has-text('Vote')").First().Click()
	bob.Locator("button:has-text('Vote')").First().Click()

	// Auto-advance to discussing — winner via SSE
	expect(t, pw.Locator(alice.Locator("text=Discussion time")).ToBeVisible())

	// No more cards — end session.
	// htmx-ext-sse has a known issue where hx-post bindings on SSE-swapped
	// content can be inert (bigskysoftware/htmx#2023). Reload ensures
	// htmx fully processes the page before clicking.
	alice.Reload()
	expect(t, pw.Locator(alice.Locator("button:has-text('End session')")).ToBeVisible())
	alice.Locator("button:has-text('End session')").Click()

	// Reload to verify server state — the finish broadcast can be missed
	// if the SSE reconnects after the reload above.
	alice.Reload()
	bob.Reload()
	expect(t, pw.Locator(alice.Locator("text=Session summary")).ToBeVisible())
	expect(t, pw.Locator(bob.Locator("text=Session summary")).ToBeVisible())
}
