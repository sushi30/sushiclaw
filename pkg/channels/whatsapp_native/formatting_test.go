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
