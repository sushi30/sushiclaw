package secureinput

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/sushi30/sushiclaw/pkg/tools/exec"
	"golang.org/x/term"
)

const scheme = "secure-input://"

var errUnavailable = errors.New("secure input unavailable")

// Store keeps sensitive values in memory, scoped to the chat/session ID.
type Store struct {
	mu     sync.RWMutex
	values map[string]map[string]string
}

func NewStore() *Store {
	return &Store{values: make(map[string]map[string]string)}
}

func (s *Store) Store(ctx context.Context, value string) (string, error) {
	if s == nil {
		return "", errUnavailable
	}
	session := sessionFromContext(ctx)
	if session == "" {
		return "", errUnavailable
	}
	token, err := randomToken()
	if err != nil {
		return "", errUnavailable
	}
	handle := scheme + session + "/" + token

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values[session] == nil {
		s.values[session] = make(map[string]string)
	}
	s.values[session][handle] = value
	return handle, nil
}

func (s *Store) Resolve(ctx context.Context, handle string) (string, error) {
	if s == nil || !IsHandle(handle) {
		return "", errUnavailable
	}
	session := sessionFromContext(ctx)
	if session == "" || !strings.HasPrefix(handle, scheme+session+"/") {
		return "", errUnavailable
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if value, ok := s.values[session][handle]; ok {
		return value, nil
	}
	return "", errUnavailable
}

func (s *Store) ClearSession(session string) {
	if s == nil || session == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, session)
}

func (s *Store) ClearAll() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values = make(map[string]map[string]string)
}

func IsHandle(value string) bool {
	return strings.HasPrefix(value, scheme)
}

type PromptFunc func(prompt string) (string, error)

type Tool struct {
	store  *Store
	prompt PromptFunc
}

func NewTool(store *Store, prompt PromptFunc) *Tool {
	return &Tool{store: store, prompt: prompt}
}

func (t *Tool) Name() string { return "secure_input" }

func (t *Tool) Description() string {
	return "Request sensitive input from the local terminal and return an opaque in-memory handle."
}

func (t *Tool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"prompt": {
			Type:        "string",
			Description: "Optional prompt shown only on the local terminal",
			Required:    false,
		},
		"name": {
			Type:        "string",
			Description: "Optional human-readable label for the captured value",
			Required:    false,
		},
	}
}

func (t *Tool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

func (t *Tool) Execute(ctx context.Context, args string) (string, error) {
	var req struct {
		Prompt string `json:"prompt"`
		Name   string `json:"name"`
	}
	if strings.TrimSpace(args) != "" {
		_ = json.Unmarshal([]byte(args), &req)
	}

	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		prompt = "Secure input"
	}
	read := t.prompt
	if read == nil {
		read = TerminalPrompt(os.Stdin, os.Stderr)
	}
	value, err := read(prompt)
	if err != nil || value == "" {
		return "", errUnavailable
	}

	handle, err := t.store.Store(ctx, value)
	if err != nil {
		return "", errUnavailable
	}

	resp := map[string]string{
		"status": "captured",
		"handle": handle,
	}
	if req.Name != "" {
		resp["name"] = req.Name
	}
	out, err := json.Marshal(resp)
	if err != nil {
		return "", errUnavailable
	}
	return string(out), nil
}

func TerminalPrompt(in *os.File, out io.Writer) PromptFunc {
	return func(prompt string) (string, error) {
		if in == nil || !term.IsTerminal(int(in.Fd())) {
			return "", errUnavailable
		}
		if out != nil {
			_, _ = fmt.Fprintf(out, "%s: ", prompt)
		}
		bytes, err := term.ReadPassword(int(in.Fd()))
		if out != nil {
			_, _ = fmt.Fprintln(out)
		}
		if err != nil {
			return "", errUnavailable
		}
		return strings.TrimRight(string(bytes), "\r\n"), nil
	}
}

func ReaderPrompt(in io.Reader, out io.Writer) PromptFunc {
	return func(prompt string) (string, error) {
		if in == nil {
			return "", errUnavailable
		}
		if out != nil {
			_, _ = fmt.Fprintf(out, "%s: ", prompt)
		}
		line, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", errUnavailable
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
}

type ResolvingTool struct {
	tool  interfaces.Tool
	store *Store
}

func Wrap(tool interfaces.Tool, store *Store) interfaces.Tool {
	if tool == nil || tool.Name() == "secure_input" {
		return tool
	}
	return &ResolvingTool{tool: tool, store: store}
}

func WrapAll(tools []interfaces.Tool, store *Store) []interfaces.Tool {
	out := make([]interfaces.Tool, len(tools))
	for i, tool := range tools {
		out[i] = Wrap(tool, store)
	}
	return out
}

func (t *ResolvingTool) Name() string { return t.tool.Name() }

func (t *ResolvingTool) Description() string { return t.tool.Description() }

func (t *ResolvingTool) Parameters() map[string]interfaces.ParameterSpec {
	return t.tool.Parameters()
}

func (t *ResolvingTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

func (t *ResolvingTool) Execute(ctx context.Context, args string) (string, error) {
	resolvedArgs, secrets, err := t.resolveArgs(ctx, args)
	if err != nil {
		return "", errUnavailable
	}
	out, runErr := t.tool.Execute(ctx, resolvedArgs)
	out = redact(out, secrets)
	if runErr != nil {
		return out, errors.New(redact(runErr.Error(), secrets))
	}
	return out, nil
}

func (t *ResolvingTool) resolveArgs(ctx context.Context, args string) (string, []string, error) {
	if IsHandle(args) {
		value, err := t.store.Resolve(ctx, args)
		return value, []string{value}, err
	}

	var v any
	if err := json.Unmarshal([]byte(args), &v); err != nil {
		return args, nil, nil
	}
	secrets := make([]string, 0)
	resolved, err := t.resolveValue(ctx, v, &secrets)
	if err != nil {
		return "", nil, err
	}
	out, err := json.Marshal(resolved)
	if err != nil {
		return "", nil, errUnavailable
	}
	return string(out), secrets, nil
}

func (t *ResolvingTool) resolveValue(ctx context.Context, value any, secrets *[]string) (any, error) {
	switch v := value.(type) {
	case string:
		if !IsHandle(v) {
			return v, nil
		}
		secret, err := t.store.Resolve(ctx, v)
		if err != nil {
			return nil, err
		}
		*secrets = append(*secrets, secret)
		return secret, nil
	case []any:
		for i := range v {
			resolved, err := t.resolveValue(ctx, v[i], secrets)
			if err != nil {
				return nil, err
			}
			v[i] = resolved
		}
		return v, nil
	case map[string]any:
		for key := range v {
			resolved, err := t.resolveValue(ctx, v[key], secrets)
			if err != nil {
				return nil, err
			}
			v[key] = resolved
		}
		return v, nil
	default:
		return v, nil
	}
}

func redact(value string, secrets []string) string {
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	return value
}

func sessionFromContext(ctx context.Context) string {
	return exec.ChatIDFromContext(ctx)
}

func randomToken() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}
