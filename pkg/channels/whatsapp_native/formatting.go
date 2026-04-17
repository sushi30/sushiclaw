package whatsapp

import (
	"regexp"
	"strings"
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
)

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
