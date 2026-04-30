// Package chat provides a terminal REPL for interacting with the sushiclaw agent.
package chat

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/sushi30/sushiclaw/internal/agent"
	"github.com/sushi30/sushiclaw/internal/commandfilter"
	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/channels"
	wschannel "github.com/sushi30/sushiclaw/pkg/channels/websocket"
	"github.com/sushi30/sushiclaw/pkg/commands"
	"github.com/sushi30/sushiclaw/pkg/config"
	sushitools "github.com/sushi30/sushiclaw/pkg/tools"
)

// ErrQuit signals the REPL should exit cleanly.
var ErrQuit = errors.New("user quit")

const chatSessionKey = "websocket:cli"

// Runner holds the REPL state.
type Runner struct {
	scanner *bufio.Scanner
	out     io.Writer
	outMu   sync.Mutex
	session *agent.SessionManager
	bus     *bus.MessageBus
	manager *channels.Manager
	conn    *websocket.Conn
	token   string
	port    int
}

// NewRunner creates a chat runner from config.
func NewRunner(cfg *config.Config) (*Runner, error) {
	tools := sushitools.NewChatTools(cfg)

	messageBus := bus.NewMessageBus()
	sessionMgr, err := agent.NewSessionManager(cfg, messageBus, tools, nil)
	if err != nil {
		return nil, fmt.Errorf("create agent session: %w", err)
	}

	cm, err := channels.NewManager(&config.Config{}, messageBus, nil)
	if err != nil {
		return nil, fmt.Errorf("create channel manager: %w", err)
	}
	messageBus.SetStreamDelegate(cm)

	token, err := randomToken()
	if err != nil {
		return nil, fmt.Errorf("generate websocket token: %w", err)
	}
	port, err := availablePort()
	if err != nil {
		return nil, fmt.Errorf("find websocket port: %w", err)
	}

	bc := &config.Channel{
		Enabled:   true,
		Type:      config.ChannelWebSocket,
		AllowFrom: config.FlexibleStringSlice{"*"},
	}
	bc.SetName(config.ChannelWebSocket)
	wsCfg := &config.WebSocketSettings{
		Port:         port,
		AllowOrigins: []string{"*"},
	}
	wsCfg.SetToken(token)
	wsChannel, err := wschannel.NewWebSocketChannel(bc, wsCfg, messageBus)
	if err != nil {
		return nil, fmt.Errorf("create websocket channel: %w", err)
	}
	cm.RegisterChannel(config.ChannelWebSocket, wsChannel)

	return &Runner{
		scanner: bufio.NewScanner(os.Stdin),
		out:     os.Stdout,
		session: sessionMgr,
		bus:     messageBus,
		manager: cm,
		token:   token,
		port:    port,
	}, nil
}

// Run starts the REPL loop.
// SetInput replaces the scanner input (for testing).
func (r *Runner) SetInput(rd io.Reader) {
	r.scanner = bufio.NewScanner(rd)
}

// SetOutput replaces the output writer (for testing).
func (r *Runner) SetOutput(w io.Writer) {
	r.out = w
}

func (r *Runner) Run(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)

	if err := r.start(runCtx); err != nil {
		cancel()
		return err
	}
	defer r.stop()
	defer cancel()

	r.println("Sushiclaw Chat")
	r.println("Type /quit to exit, /help for commands")
	r.println("")

	for {
		r.print("> ")
		if !r.scanner.Scan() {
			break
		}

		line := r.scanner.Text()
		if line == "" {
			continue
		}

		// Handle REPL commands
		if handled, err := r.handleCommand(ctx, line); err != nil {
			if errors.Is(err, ErrQuit) {
				return nil
			}
			return err
		} else if handled {
			continue
		}

		if err := r.send(line); err != nil {
			r.printf("Error: %v\n", err)
			continue
		}
	}

	return r.scanner.Err()
}

func (r *Runner) handleCommand(ctx context.Context, line string) (bool, error) {
	_ = ctx
	switch line {
	case "/quit", "/q", "/exit":
		r.println("Goodbye!")
		return true, ErrQuit
	case "/clear":
		if r.session != nil {
			if err := r.session.ClearHistory(chatSessionKey); err != nil {
				return true, err
			}
		}
		r.println("History cleared.")
		return true, nil
	case "/help", "/h":
		r.println("Commands:")
		r.println("  /quit    Exit the REPL")
		r.println("  /clear   Clear conversation history")
		r.println("  /help    Show this help")
		return true, nil
	}
	return false, nil
}

func (r *Runner) start(ctx context.Context) error {
	if err := r.manager.StartAll(ctx); err != nil {
		return fmt.Errorf("start websocket channel: %w", err)
	}

	r.startInboundLoop(ctx)

	conn, err := r.dial(ctx)
	if err != nil {
		_ = r.manager.StopAll(context.Background())
		return err
	}
	r.conn = conn

	go r.readLoop(ctx)
	return nil
}

func (r *Runner) stop() {
	if r.conn != nil {
		_ = r.conn.Close()
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = r.manager.StopAll(shutdownCtx)
	if r.bus != nil {
		r.bus.Close()
	}
}

func (r *Runner) startInboundLoop(ctx context.Context) {
	cmdFilter := commandfilter.NewCommandFilter()
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	rt := &commands.Runtime{
		ListDefinitions: reg.Definitions,
		GetModelInfo:    r.session.GetModelInfo,
		ListModels:      r.session.ListModels,
		ListSkills:      r.session.ListSkills,
		ClearHistory: func(req commands.Request) error {
			return r.session.ClearHistory(req.SessionKey)
		},
		ActivateSkill: func(req commands.Request, skillName string) error {
			return r.session.ActivateSkill(req.SessionKey, skillName)
		},
	}
	executor := commands.NewExecutor(reg, rt)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-r.bus.InboundChan():
				if !ok {
					return
				}
				dec := cmdFilter.Filter(msg)
				if dec.Result == commandfilter.Block {
					_ = r.bus.PublishOutbound(ctx, bus.OutboundMessage{
						Channel: msg.Channel,
						ChatID:  msg.ChatID,
						Content: dec.ErrMsg,
					})
					continue
				}

				sessionKey := msg.SessionKey
				if sessionKey == "" {
					sessionKey = msg.Channel + ":" + msg.ChatID
				}

				if commands.HasCommandPrefix(msg.Content) {
					var reply string
					result := executor.Execute(ctx, commands.Request{
						Channel:    msg.Channel,
						ChatID:     msg.ChatID,
						SenderID:   msg.SenderID,
						Text:       msg.Content,
						SessionKey: sessionKey,
						Reply:      func(text string) error { reply = text; return nil },
					})
					if result.Outcome == commands.OutcomeHandled {
						if reply != "" {
							_ = r.bus.PublishOutbound(ctx, bus.OutboundMessage{
								Channel: msg.Channel,
								ChatID:  msg.ChatID,
								Content: reply,
							})
						}
						continue
					}
				}

				go r.session.Dispatch(ctx, msg)
			}
		}
	}()
}

func (r *Runner) dial(ctx context.Context) (*websocket.Conn, error) {
	url := fmt.Sprintf("ws://127.0.0.1:%d/websocket/ws?session_id=cli", r.port)
	header := http.Header{"Authorization": {"Bearer " + r.token}}

	var lastErr error
	for range 50 {
		conn, resp, err := websocket.DefaultDialer.DialContext(ctx, url, header)
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if err == nil {
			return conn, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
	return nil, fmt.Errorf("connect websocket channel: %w", lastErr)
}

func (r *Runner) send(line string) error {
	if r.conn == nil {
		return errors.New("websocket is not connected")
	}
	msg := wschannel.WebSocketMessage{
		Type:      wschannel.TypeMessageSend,
		SessionID: "cli",
		Payload: map[string]any{
			wschannel.PayloadKeyContent: line,
		},
	}
	return r.conn.WriteJSON(msg)
}

func (r *Runner) readLoop(ctx context.Context) {
	for {
		var msg wschannel.WebSocketMessage
		if err := r.conn.ReadJSON(&msg); err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				r.printf("\nWebSocket closed: %v\n", err)
				return
			}
		}

		switch msg.Type {
		case wschannel.TypeMessageCreate, wschannel.TypeMessageUpdate:
			content, _ := msg.Payload[wschannel.PayloadKeyContent].(string)
			if content != "" {
				r.printf("\n%s\n", content)
			}
		case wschannel.TypeError:
			message, _ := msg.Payload["message"].(string)
			if message != "" {
				r.printf("\nError: %s\n", message)
			}
		}
	}
}

func (r *Runner) print(s string) {
	r.outMu.Lock()
	defer r.outMu.Unlock()
	_, _ = fmt.Fprint(r.out, s)
}

func (r *Runner) println(s string) {
	r.outMu.Lock()
	defer r.outMu.Unlock()
	_, _ = fmt.Fprintln(r.out, s)
}

func (r *Runner) printf(format string, args ...any) {
	r.outMu.Lock()
	defer r.outMu.Unlock()
	_, _ = fmt.Fprintf(r.out, format, args...)
}

func randomToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func availablePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = ln.Close() }()

	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address %T", ln.Addr())
	}
	return addr.Port, nil
}
