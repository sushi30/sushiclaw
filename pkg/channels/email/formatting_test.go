package email

import (
	"strings"
	"testing"
)

func TestMarkdownToHTML(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantContain string
	}{
		{
			name:        "empty",
			input:       "",
			wantContain: "",
		},
		{
			name:        "heading",
			input:       "# Hello",
			wantContain: "<h1>Hello</h1>",
		},
		{
			name:        "bold",
			input:       "**important**",
			wantContain: "<strong>important</strong>",
		},
		{
			name:        "bullet list",
			input:       "- one\n- two",
			wantContain: "<ul>",
		},
		{
			name:        "bullet list items",
			input:       "- one\n- two",
			wantContain: "<li>one</li>",
		},
		{
			name:        "code block",
			input:       "```\nfunc foo() {}\n```",
			wantContain: "<pre>",
		},
		{
			name:        "inline code",
			input:       "Use `go test` to run",
			wantContain: "<code>go test</code>",
		},
		{
			name:        "plain paragraph",
			input:       "Hello world",
			wantContain: "<p>Hello world</p>",
		},
		{
			name:        "link",
			input:       "[click here](https://example.com)",
			wantContain: `href="https://example.com"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := markdownToHTML(tc.input)
			if tc.wantContain == "" {
				if strings.TrimSpace(got) != "" {
					t.Errorf("markdownToHTML(%q) = %q, want empty", tc.input, got)
				}
				return
			}
			if !strings.Contains(got, tc.wantContain) {
				t.Errorf("markdownToHTML(%q)\n got:  %q\nmissing: %q", tc.input, got, tc.wantContain)
			}
		})
	}
}
