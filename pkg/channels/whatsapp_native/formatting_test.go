package whatsapp

import (
	"testing"
)

func TestStripMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty",
			input: "",
			want:  "",
		},
		{
			name:  "plain text unchanged",
			input: "Hello world",
			want:  "Hello world",
		},
		{
			name:  "h1 heading",
			input: "# Title",
			want:  "Title",
		},
		{
			name:  "h2 heading",
			input: "## Section",
			want:  "Section",
		},
		{
			name:  "h3 heading",
			input: "### Sub",
			want:  "Sub",
		},
		{
			name:  "bold star",
			input: "**bold text**",
			want:  "bold text",
		},
		{
			name:  "bold underscore",
			input: "__bold text__",
			want:  "bold text",
		},
		{
			name:  "italic underscore",
			input: "_italic_",
			want:  "italic",
		},
		{
			name:  "strikethrough",
			input: "~~struck~~",
			want:  "struck",
		},
		{
			name:  "inline code",
			input: "`code here`",
			want:  "code here",
		},
		{
			name:  "code block",
			input: "```\nfunc foo() {}\n```",
			want:  "func foo() {}",
		},
		{
			name:  "code block with language",
			input: "```go\nfunc foo() {}\n```",
			want:  "func foo() {}",
		},
		{
			name:  "link",
			input: "[click here](https://example.com)",
			want:  "click here",
		},
		{
			name:  "list item dash",
			input: "- item one",
			want:  "item one",
		},
		{
			name:  "list item star",
			input: "* item two",
			want:  "item two",
		},
		{
			name:  "mixed LLM response",
			input: "## Summary\n\n**Key points:**\n\n- First item with `code`\n- [link](https://x.com)\n- ~~old~~ new",
			want:  "Summary\n\nKey points:\n\nFirst item with code\nlink\nold new",
		},
		{
			name:  "multiline with heading and bold",
			input: "# Hello\n\nThis is **important**.",
			want:  "Hello\n\nThis is important.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := stripMarkdown(tc.input)
			if got != tc.want {
				t.Errorf("stripMarkdown(%q)\n got:  %q\nwant: %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestDetectOptions(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantBody string
		wantOpts []string
	}{
		{
			name:     "numbered list with question",
			input:    "Which list?\n1. Backlog\n2. In Progress\n3. Done",
			wantBody: "Which list?",
			wantOpts: []string{"Backlog", "In Progress", "Done"},
		},
		{
			name:     "bulleted list with colon",
			input:    "Pick a priority:\n- High\n- Medium\n- Low",
			wantBody: "Pick a priority:",
			wantOpts: []string{"High", "Medium", "Low"},
		},
		{
			name:     "4 options exceeds button limit -> list",
			input:    "Select a list?\n1. Backlog\n2. In Progress\n3. Done\n4. Archive",
			wantBody: "Select a list?",
			wantOpts: []string{"Backlog", "In Progress", "Done", "Archive"},
		},
		{
			name:     "numbered with paren",
			input:    "Choose one?\n1) Alpha\n2) Beta",
			wantBody: "Choose one?",
			wantOpts: []string{"Alpha", "Beta"},
		},
		{
			name:     "bullet with star",
			input:    "Which color?\n* Red\n* Blue",
			wantBody: "Which color?",
			wantOpts: []string{"Red", "Blue"},
		},
		{
			name:     "bullet with unicode bullet",
			input:    "Pick one?\n• First\n• Second",
			wantBody: "Pick one?",
			wantOpts: []string{"First", "Second"},
		},
		{
			name:     "plain text no list",
			input:    "Hello world, how are you?",
			wantOpts: nil,
		},
		{
			name:     "body without ? or : not detected",
			input:    "Here is a list\n1. Alpha\n2. Beta",
			wantOpts: nil,
		},
		{
			name:     "single item not detected",
			input:    "Pick one?\n1. Only option",
			wantOpts: nil,
		},
		{
			name:     "11 items exceeds maximum not detected",
			input:    "Which one?\n1. A\n2. B\n3. C\n4. D\n5. E\n6. F\n7. G\n8. H\n9. I\n10. J\n11. K",
			wantOpts: nil,
		},
		{
			name:     "multiline body trimmed",
			input:    "I found these options.\nWhich would you like?\n- Apple\n- Banana",
			wantBody: "I found these options.\nWhich would you like?",
			wantOpts: []string{"Apple", "Banana"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, opts := detectOptions(tc.input)
			if len(opts) == 0 && len(tc.wantOpts) == 0 {
				return // both nil/empty: pass
			}
			if len(opts) != len(tc.wantOpts) {
				t.Fatalf("opts count: got %d %v, want %d %v", len(opts), opts, len(tc.wantOpts), tc.wantOpts)
			}
			if body != tc.wantBody {
				t.Errorf("body: got %q, want %q", body, tc.wantBody)
			}
			for i, o := range opts {
				if o != tc.wantOpts[i] {
					t.Errorf("opts[%d]: got %q, want %q", i, o, tc.wantOpts[i])
				}
			}
		})
	}
}
