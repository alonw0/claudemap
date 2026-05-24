package analyze

import (
	"testing"
)

func TestStripHTMLComments(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "strips comment",
			input: "before <!-- a comment --> after",
			want:  "before  after",
		},
		{
			name:  "preserves comment inside code block",
			input: "text\n```\n<!-- inside code -->\n```\nmore",
			want:  "text\n```\n<!-- inside code -->\n```\nmore",
		},
		{
			name:  "strips multiline comment",
			input: "a\n<!--\nmulti\nline\n-->\nb",
			want:  "a\n\nb",
		},
		{
			name:  "no comment",
			input: "plain text",
			want:  "plain text",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripHTMLComments(tt.input)
			if got != tt.want {
				t.Errorf("StripHTMLComments() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		minPct  float64
		maxPct  float64
		approx  int
	}{
		{
			name:   "empty",
			input:  "",
			approx: 0,
		},
		{
			name:   "prose paragraph",
			input:  "Always write clear commit messages. Explain the why, not just the what. Keep lines under 72 characters.",
			approx: 22,
		},
		{
			name:   "code block",
			input:  "```\nfunction hello() {\n  console.log('hello world');\n}\n```",
			approx: 17,
		},
		{
			name:   "html comment excluded",
			input:  "real content <!-- this is a comment --> more content",
			approx: 8, // comment stripped
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EstimateTokens(tt.input)
			if tt.approx == 0 {
				if got != 0 {
					t.Errorf("EstimateTokens(%q) = %d, want 0", tt.input, got)
				}
				return
			}
			// Allow ±30% for heuristic estimates
			lo := int(float64(tt.approx) * 0.7)
			hi := int(float64(tt.approx) * 1.3)
			if got < lo || got > hi {
				t.Errorf("EstimateTokens() = %d, want ~%d (±30%%)", got, tt.approx)
			}
		})
	}
}
