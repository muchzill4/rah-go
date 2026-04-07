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

func (s *Server) renderNotFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	err := templates.ExecuteTemplate(w, "notfound.html", nil)
	if err != nil {
		slog.Error("template render failed", "err", err)
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
