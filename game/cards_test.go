package game

import "testing"

func TestNormalizeCardText(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello {blank} world", "Hello {blank} world"},
		{"Hello _ world", "Hello {blank} world"},
		{"Hello __ world", "Hello {blank} world"},
		{"Hello {anything} world", "Hello {blank} world"},
		{"Hello {BLANK} world", "Hello {blank} world"},
		{"{_} and {stuff}", "{blank} and {blank}"},
	}

	for _, tt := range tests {
		got := NormalizeCardText(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeCardText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFillInBlank(t *testing.T) {
	tests := []struct {
		card   string
		answer string
		want   string
	}{
		{"Hello {blank} world", "Go", "Hello Go world"},
		{"Between {blank} and {blank}", "cats|||dogs", "Between cats and dogs"},
		{"Just {blank}", "one", "Just one"},
	}

	for _, tt := range tests {
		got := FillInBlank(tt.card, tt.answer)
		if got != tt.want {
			t.Errorf("FillInBlank(%q, %q) = %q, want %q", tt.card, tt.answer, got, tt.want)
		}
	}
}

func TestBlankCount(t *testing.T) {
	if BlankCount("no blanks") != 0 {
		t.Fatal("want 0")
	}
	if BlankCount("one {blank}") != 1 {
		t.Fatal("want 1")
	}
	if BlankCount("{blank} and {blank}") != 2 {
		t.Fatal("want 2")
	}
}
