package whatsapp

import (
	"regexp"
	"strings"
)

const (
	waMaxButtonOptions = 3
	waMaxListOptions   = 10
)

var (
	waReHeading    = regexp.MustCompile(`(?m)^#{1,6}\s+([^\n]+)`)
	waReBlockquote = regexp.MustCompile(`(?m)^>\s*(.*)$`)
	waReLink       = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	waReBoldStar   = regexp.MustCompile(`\*\*(.+?)\*\*`)
	waReBoldUnder  = regexp.MustCompile(`__(.+?)__`)
	waReItalic     = regexp.MustCompile(`_([^_]+)_`)
	waReStrike     = regexp.MustCompile(`~~(.+?)~~`)
	waReListItem   = regexp.MustCompile(`(?m)^[-*]\s+`)
	waReCodeBlock  = regexp.MustCompile("(?s)```[\\w]*\\n?([\\s\\S]*?)```")
	waReInlineCode = regexp.MustCompile("`([^`]+)`")

	// waReOptionItem matches a numbered (1. / 1)) or bulleted (- / * / •) list item at line start.
	waReOptionItem = regexp.MustCompile(`(?m)^(?:\d+[.)]\s+|[-*•]\s+)(.+)`)
)

// detectOptions inspects content for a decision-point pattern: an introductory
// sentence ending with "?" or ":" followed by 2–10 list items. Returns the body
// text and option labels when the pattern matches; both are empty otherwise.
func detectOptions(content string) (body string, opts []string) {
	matches := waReOptionItem.FindAllStringSubmatchIndex(content, -1)
	if len(matches) < 2 || len(matches) > waMaxListOptions {
		return
	}
	firstStart := matches[0][0]
	rawBody := strings.TrimSpace(content[:firstStart])
	if !strings.HasSuffix(rawBody, "?") && !strings.HasSuffix(rawBody, ":") {
		return
	}
	body = rawBody
	for _, m := range matches {
		opt := strings.TrimSpace(content[m[2]:m[3]])
		if opt != "" {
			opts = append(opts, opt)
		}
	}
	return
}

func stripMarkdown(s string) string {
	s = waReCodeBlock.ReplaceAllStringFunc(s, func(m string) string {
		parts := waReCodeBlock.FindStringSubmatch(m)
		if len(parts) < 2 {
			return m
		}
		return strings.TrimSpace(parts[1])
	})
	s = waReInlineCode.ReplaceAllString(s, "$1")
	s = waReHeading.ReplaceAllString(s, "$1")
	s = waReBlockquote.ReplaceAllString(s, "$1")
	s = waReLink.ReplaceAllString(s, "$1")
	s = waReBoldStar.ReplaceAllString(s, "$1")
	s = waReBoldUnder.ReplaceAllString(s, "$1")
	s = waReItalic.ReplaceAllString(s, "$1")
	s = waReStrike.ReplaceAllString(s, "$1")
	s = waReListItem.ReplaceAllString(s, "")
	return s
}
