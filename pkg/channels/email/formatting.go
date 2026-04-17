package email

import (
	"strings"

	"github.com/gomarkdown/markdown"
	mdhtml "github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

func markdownToHTML(md string) string {
	if strings.TrimSpace(md) == "" {
		return ""
	}
	extensions := (parser.CommonExtensions | parser.NoEmptyLineBeforeBlock) &^ parser.DefinitionLists
	p := parser.NewWithExtensions(extensions)
	renderer := mdhtml.NewRenderer(mdhtml.RendererOptions{Flags: mdhtml.UseXHTML})
	return strings.TrimSpace(string(markdown.ToHTML([]byte(md), p, renderer)))
}
