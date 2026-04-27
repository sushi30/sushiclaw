package agent

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sushi30/sushiclaw/pkg/commands"
	"github.com/sushi30/sushiclaw/pkg/logger"
)

// ContextBuilder assembles the agent system prompt from workspace files:
// AGENT.md, IDENTITY.md, SOUL.md, USER.md, and skills/*/SKILL.md.
// Prompts are cached and invalidated when source file mtimes change.
type ContextBuilder struct {
	workspace string

	mu             sync.RWMutex
	cachedPrompt   string
	fileBaseline   map[string]time.Time
	existedAtCache map[string]bool
}

// NewContextBuilder creates a ContextBuilder for the given workspace directory.
func NewContextBuilder(workspace string) *ContextBuilder {
	return &ContextBuilder{workspace: workspace}
}

// BuildSystemPromptWithCache returns the assembled system prompt, rebuilding
// only when source files have changed since the last build.
func (b *ContextBuilder) BuildSystemPromptWithCache() (string, error) {
	b.mu.RLock()
	if b.cachedPrompt != "" && b.cacheValidLocked() {
		p := b.cachedPrompt
		b.mu.RUnlock()
		return p, nil
	}
	b.mu.RUnlock()

	b.mu.Lock()
	defer b.mu.Unlock()

	// Double-checked: another goroutine may have rebuilt while we waited.
	if b.cachedPrompt != "" && b.cacheValidLocked() {
		return b.cachedPrompt, nil
	}

	baseline, existed := b.captureBaseline()

	prompt, err := b.buildPrompt()
	if err != nil {
		return "", err
	}

	b.cachedPrompt = prompt
	b.fileBaseline = baseline
	b.existedAtCache = existed
	return prompt, nil
}

// buildPrompt reads workspace files and assembles the system prompt.
func (b *ContextBuilder) buildPrompt() (string, error) {
	var sections []string

	agentBody, agentOK := b.readFileIfExists(filepath.Join(b.workspace, "AGENT.md"))
	if agentOK {
		if body := parseMarkdownBody(agentBody); body != "" {
			sections = append(sections, body)
		}
	}

	if identity, ok := b.readFirstExistingFile(
		filepath.Join(b.workspace, "IDENTITY.md"),
		filepath.Join(b.workspace, "USER.md"),
	); ok {
		sections = append(sections, "## Identity\n\n"+identity)
	}

	if soul, ok := b.readFileIfExists(filepath.Join(b.workspace, "SOUL.md")); ok {
		sections = append(sections, soul)
	}

	if summary := b.skillsSummary(filepath.Join(b.workspace, "skills")); summary != "" {
		sections = append(sections, summary)
	}

	if len(sections) == 0 {
		logger.DebugC("agent", "No workspace context files found; using default system prompt")
		return "", nil
	}

	sections = append([]string{workspaceEntrypointsSection()}, sections...)
	return strings.Join(sections, "\n\n---\n\n"), nil
}

// cacheValidLocked checks whether all tracked files match the cached baseline.
// Must be called with at least a read lock held.
func (b *ContextBuilder) cacheValidLocked() bool {
	if b.fileBaseline == nil {
		return false
	}
	for path, cachedMtime := range b.fileBaseline {
		info, err := os.Stat(path)
		if err != nil {
			if b.existedAtCache[path] {
				return false // file was deleted
			}
			continue
		}
		if !b.existedAtCache[path] {
			return false // file was created
		}
		if info.ModTime().After(cachedMtime) {
			return false // file was modified
		}
	}
	// Also check for new skill files not in baseline.
	skillsDir := filepath.Join(b.workspace, "skills")
	entries, err := os.ReadDir(skillsDir)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			p := filepath.Join(skillsDir, e.Name(), "SKILL.md")
			if _, tracked := b.fileBaseline[p]; !tracked {
				return false // new skill appeared
			}
		}
	}
	return true
}

// captureBaseline records the current mtime and existence of all source files.
func (b *ContextBuilder) captureBaseline() (map[string]time.Time, map[string]bool) {
	paths := []string{
		filepath.Join(b.workspace, "AGENT.md"),
		filepath.Join(b.workspace, "IDENTITY.md"),
		filepath.Join(b.workspace, "SOUL.md"),
		filepath.Join(b.workspace, "USER.md"),
	}

	skillsDir := filepath.Join(b.workspace, "skills")
	if entries, err := os.ReadDir(skillsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				paths = append(paths, filepath.Join(skillsDir, e.Name(), "SKILL.md"))
			}
		}
	}

	baseline := make(map[string]time.Time, len(paths))
	existed := make(map[string]bool, len(paths))
	for _, p := range paths {
		info, err := os.Stat(p)
		if err == nil {
			baseline[p] = info.ModTime()
			existed[p] = true
		} else {
			baseline[p] = time.Time{}
			existed[p] = false
		}
	}
	return baseline, existed
}

// skillsSummary walks skills/ and builds a markdown summary block.
func (b *ContextBuilder) skillsSummary(skillsDir string) string {
	skills := listSkillsInDir(skillsDir)
	if len(skills) == 0 {
		return ""
	}

	lines := make([]string, 0, len(skills))
	for _, skill := range skills {
		lines = append(lines, "- **"+skill.Name+"**: "+skill.Description)
	}

	return "## Skills\n\n" + strings.Join(lines, "\n")
}

func listSkillsInDir(skillsDir string) []commands.SkillInfo {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil
	}

	skills := make([]commands.SkillInfo, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillFile := filepath.Join(skillsDir, e.Name(), "SKILL.md")
		content, err := os.ReadFile(skillFile)
		if err != nil {
			continue
		}
		desc := skillDescription(string(content))
		if desc == "" {
			desc = e.Name()
		}
		skills = append(skills, commands.SkillInfo{
			Name:        e.Name(),
			Description: desc,
		})
	}
	return skills
}

// readFileIfExists returns the file content and whether the file existed.
func (b *ContextBuilder) readFileIfExists(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(data)), true
}

func (b *ContextBuilder) readFirstExistingFile(paths ...string) (string, bool) {
	for _, path := range paths {
		if content, ok := b.readFileIfExists(path); ok {
			return content, true
		}
	}
	return "", false
}

func workspaceEntrypointsSection() string {
	return strings.TrimSpace(`## Workspace entrypoints

- ` + "`AGENT.md`" + `: authoritative source for role, mission, capabilities, tool scope, and other assistant-level behavior.
- ` + "`IDENTITY.md`" + `: authoritative source for identity, profile details, naming, and stable preferences.
- ` + "`SOUL.md`" + `: authoritative source for personality, tone, and communication style.
- ` + "`USER.md`" + `: legacy alias for ` + "`IDENTITY.md`" + `; use only when a workspace has not been migrated yet.

When a user asks you to change how the assistant behaves, infer the correct entrypoint from the requested change and edit that file directly. Do not ask the user which workspace file to touch.`)
}

// parseMarkdownBody strips YAML frontmatter (--- ... ---) from markdown content
// and returns only the body.
func parseMarkdownBody(content string) string {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return content
	}
	// Skip the opening ---
	rest := content[3:]
	// Find the closing ---
	idx := strings.Index(rest, "\n---")
	if idx == -1 {
		return content
	}
	body := strings.TrimSpace(rest[idx+4:])
	return body
}

func skillDescription(content string) string {
	if desc := frontmatterDescription(content); desc != "" {
		return desc
	}
	return firstDescriptionLine(content)
}

func frontmatterDescription(content string) string {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return ""
	}
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx == -1 {
		return ""
	}
	for _, line := range strings.Split(rest[:idx], "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok || strings.TrimSpace(key) != "description" {
			continue
		}
		return strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return ""
}

// firstDescriptionLine returns the first non-blank, non-frontmatter line from
// a SKILL.md file suitable for use as a one-line description.
func firstDescriptionLine(content string) string {
	body := parseMarkdownBody(content)
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Strip leading markdown list/emphasis markers for cleanliness.
		line = strings.TrimLeft(line, "-*> ")
		if line != "" {
			return line
		}
	}
	return ""
}
