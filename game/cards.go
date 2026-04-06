package game

import (
	"regexp"
	"strings"
)

const BlankPlaceholder = "{blank}"

var DefaultCards = []string{
	"If history has taught us anything, it's to never underestimate {blank}.",
	"We must strive for a balance between {blank} and {blank}.",
	"Everybody in the office seems to be obsessed with {blank} these days.",
	"Our team's spirit animal is clearly {blank}.",
	"The hidden cost of {blank} is always {blank}.",
	"If our project was a sandwich, {blank} would be the secret sauce.",
	"Our standup meetings would be more productive if we {blank}.",
	"The best way to onboard new team members is {blank}.",
	"In our next retrospective, we need to seriously discuss {blank}.",
	"The thing nobody wants to talk about is {blank}.",
	"This sprint taught us about {blank}.",
	"We could be doing more {blank}.",
	"{blank} is an experiment worth trying.",
	"The highlight of this iteration was {blank}.",
	"Major props to {blank} for {blank}.",
	"An interesting challenge was {blank}.",
	"{blank} is worth reflecting on.",
	"{blank} is one thing worth changing.",
	"The surprise of this sprint was {blank}.",
	"The glue holding the team together is {blank}.",
	"{blank} is worth investing more time in.",
}

func NormalizeCardText(text string) string {
	// Replace {anything} with {blank}
	re := regexp.MustCompile(`\{[^}]+\}`)
	text = re.ReplaceAllString(text, BlankPlaceholder)

	// Replace sequences of underscores with {blank}
	reUnder := regexp.MustCompile(`_+`)
	text = reUnder.ReplaceAllString(text, BlankPlaceholder)

	return text
}

func FillInBlank(cardText string, answer string) string {
	answers := strings.Split(answer, "|||")
	result := cardText
	for _, a := range answers {
		result = strings.Replace(result, BlankPlaceholder, a, 1)
	}
	return result
}

func BlankCount(cardText string) int {
	return strings.Count(cardText, BlankPlaceholder)
}
