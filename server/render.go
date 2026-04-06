package server

import (
	"bytes"
	"embed"
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"github.com/muchzill4/rah-go/game"
)

//go:embed templates/*.html
var templateFS embed.FS

var funcMap = template.FuncMap{
	"fillInBlank": func(cardText, answer string) template.HTML {
		answers := strings.Split(answer, "|||")
		result := cardText
		for _, a := range answers {
			result = strings.Replace(result, game.BlankPlaceholder, `<span class="filled-blank">`+template.HTMLEscapeString(a)+"</span>", 1)
		}
		return template.HTML(result)
	},
	"cardSegments": func(text string) []string {
		return strings.Split(text, game.BlankPlaceholder)
	},
	"sub": func(a, b int) int {
		return a - b
	},
	"isCurrentParticipant": func(participant *game.Participant, other game.Participant) bool {
		return participant != nil && participant.ID == other.ID
	},
	"mySubmission": func(sess *game.Session, participantID string) *game.Submission {
		for i, s := range sess.Submissions {
			if s.CardID == sess.CurrentCard.ID && s.ParticipantID == participantID {
				return &sess.Submissions[i]
			}
		}
		return nil
	},
	"blankCount": game.BlankCount,
	"submittedCount": func(sess *game.Session) int {
		count := 0
		seen := map[string]bool{}
		for _, s := range sess.Submissions {
			if s.CardID == sess.CurrentCard.ID && !seen[s.ParticipantID] {
				seen[s.ParticipantID] = true
				count++
			}
		}
		return count
	},
	"votedCount": func(sess *game.Session) int {
		count := 0
		seen := map[string]bool{}
		for _, v := range sess.Votes {
			for _, s := range sess.Submissions {
				if s.ID == v.SubmissionID && s.CardID == sess.CurrentCard.ID && !seen[v.ParticipantID] {
					seen[v.ParticipantID] = true
					count++
				}
			}
		}
		return count
	},
	"hasSubmitted": func(sess *game.Session, participantID string) bool {
		for _, s := range sess.Submissions {
			if s.ParticipantID == participantID && s.CardID == sess.CurrentCard.ID {
				return true
			}
		}
		return false
	},
	"hasVoted": func(sess *game.Session, participantID string) bool {
		for _, v := range sess.Votes {
			for _, s := range sess.Submissions {
				if s.ID == v.SubmissionID && s.CardID == sess.CurrentCard.ID && v.ParticipantID == participantID {
					return true
				}
			}
		}
		return false
	},
	"voteCount": func(sess *game.Session, submissionID string) int {
		count := 0
		for _, v := range sess.Votes {
			if v.SubmissionID == submissionID {
				count++
			}
		}
		return count
	},
	"currentSubmissions": func(sess *game.Session) []game.Submission {
		var subs []game.Submission
		for _, s := range sess.Submissions {
			if s.CardID == sess.CurrentCard.ID {
				subs = append(subs, s)
			}
		}
		return subs
	},
	"participantName": func(sess *game.Session, participantID string) string {
		for _, p := range sess.Participants {
			if p.ID == participantID {
				return p.Name
			}
		}
		return ""
	},
	"winningSubmission": func(sess *game.Session) *game.Submission {
		winner, _ := game.WinningSubmission(*sess)
		return winner
	},
	"tiedSubmissions": func(sess *game.Session) []game.Submission {
		_, tied := game.WinningSubmission(*sess)
		return tied
	},
	"undrawnCount": func(sess *game.Session) int {
		return len(sess.Cards) - len(sess.DrawnCardIDs)
	},
	"drawnCards": func(sess *game.Session) []game.Card {
		var cards []game.Card
		for _, id := range drawnCardOrder(sess) {
			for _, c := range sess.Cards {
				if c.ID == id {
					cards = append(cards, c)
				}
			}
		}
		return cards
	},
	"winnerForCard": func(sess *game.Session, cardID string) *game.Submission {
		for _, s := range sess.Submissions {
			if s.CardID == cardID && s.Winner {
				return &s
			}
		}
		return nil
	},
	"hasSubmissions": func(sess *game.Session, cardID string) bool {
		for _, s := range sess.Submissions {
			if s.CardID == cardID {
				return true
			}
		}
		return false
	},
	"votedFor": func(sess *game.Session, participantID string) string {
		for _, v := range sess.Votes {
			for _, s := range sess.Submissions {
				if s.ID == v.SubmissionID && s.CardID == sess.CurrentCard.ID && v.ParticipantID == participantID {
					return v.SubmissionID
				}
			}
		}
		return ""
	},
}

func drawnCardOrder(sess *game.Session) []string {
	var ids []string
	for id := range sess.DrawnCardIDs {
		ids = append(ids, id)
	}
	return ids
}

func init() {
	templates = template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html"))
}

var templates *template.Template

func (s *Server) render(w http.ResponseWriter, name string, data map[string]any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err := templates.ExecuteTemplate(w, name+".html", data)
	if err != nil {
		slog.Error("template render failed", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func (s *Server) renderPartial(name string, data map[string]any) string {
	var buf bytes.Buffer
	err := templates.ExecuteTemplate(&buf, name+".html", data)
	if err != nil {
		slog.Error("template render failed", "err", err)
		return ""
	}
	return buf.String()
}
