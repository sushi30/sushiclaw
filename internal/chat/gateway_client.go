package chat

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	wschannel "github.com/sushi30/sushiclaw/pkg/channels/websocket"
)

// GatewayClientOptions configures a terminal chat client for an existing gateway.
type GatewayClientOptions struct {
	Host      string
	Port      int
	Token     string
	SessionID string
}

// GatewayClientRunner is a terminal REPL backed by an existing gateway WebSocket.
type GatewayClientRunner struct {
	scanner *bufio.Scanner
	out     io.Writer
	outMu   sync.Mutex
	conn    *websocket.Conn
	opts    GatewayClientOptions
}

// NewGatewayClientRunner creates a terminal chat client for a gateway WebSocket.
func NewGatewayClientRunner(opts GatewayClientOptions) (*GatewayClientRunner, error) {
	opts.Host = strings.TrimSpace(opts.Host)
	opts.Token = strings.TrimSpace(opts.Token)
	opts.SessionID = strings.TrimSpace(opts.SessionID)
	if opts.Host == "" {
		return nil, errors.New("gateway host is required")
	}
	if opts.Token == "" {
		return nil, errors.New("gateway websocket token is required")
	}
	if opts.SessionID == "" {
		opts.SessionID = "cli"
	}

	return &GatewayClientRunner{
		scanner: bufio.NewScanner(os.Stdin),
		out:     os.Stdout,
		opts:    opts,
	}, nil
}

func (r *GatewayClientRunner) Run(ctx context.Context) error {
	conn, err := r.dial(ctx)
	if err != nil {
		return err
	}
	r.conn = conn
	defer func() { _ = r.conn.Close() }()

	r.println("Sushiclaw Gateway Chat")
	r.println("Type /quit to exit")
	r.println("")

	go r.readLoop(ctx)

	for {
		r.print("> ")
		if !r.scanner.Scan() {
			break
		}

		line := r.scanner.Text()
		switch strings.TrimSpace(line) {
		case "":
			continue
		case "/quit", "/q", "/exit":
			r.println("Goodbye!")
			return nil
		}

		if err := r.send(line); err != nil {
			r.printf("Error: %v\n", err)
			continue
		}
	}

	return r.scanner.Err()
}

func (r *GatewayClientRunner) dial(ctx context.Context) (*websocket.Conn, error) {
	wsURL, err := gatewayWebSocketURL(r.opts)
	if err != nil {
		return nil, err
	}

	header := http.Header{"Authorization": {"Bearer " + r.opts.Token}}
	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, wsURL, header)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("connect gateway websocket: %w", err)
	}
	return conn, nil
}

func (r *GatewayClientRunner) send(line string) error {
	msg := wschannel.WebSocketMessage{
		Type:      wschannel.TypeMessageSend,
		SessionID: r.opts.SessionID,
		Timestamp: time.Now().UnixMilli(),
		Payload: map[string]any{
			wschannel.PayloadKeyContent: line,
		},
	}
	return r.conn.WriteJSON(msg)
}

func (r *GatewayClientRunner) readLoop(ctx context.Context) {
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
			if wschannelPayloadThought(msg.Payload) {
				continue
			}
			content, _ := msg.Payload[wschannel.PayloadKeyContent].(string)
			if content != "" {
				r.printf("\n%s\n", content)
			}
		case wschannel.TypeTypingStart:
			r.printf("\n...\n")
		case wschannel.TypeError:
			message, _ := msg.Payload["message"].(string)
			if message != "" {
				r.printf("\nError: %s\n", message)
			}
		}
	}
}

func (r *GatewayClientRunner) print(s string) {
	r.outMu.Lock()
	defer r.outMu.Unlock()
	_, _ = fmt.Fprint(r.out, s)
}

func (r *GatewayClientRunner) println(s string) {
	r.outMu.Lock()
	defer r.outMu.Unlock()
	_, _ = fmt.Fprintln(r.out, s)
}

func (r *GatewayClientRunner) printf(format string, args ...any) {
	r.outMu.Lock()
	defer r.outMu.Unlock()
	_, _ = fmt.Fprintf(r.out, format, args...)
}

func gatewayWebSocketURL(opts GatewayClientOptions) (string, error) {
	host := strings.TrimSpace(opts.Host)
	if strings.HasPrefix(host, "ws://") || strings.HasPrefix(host, "wss://") ||
		strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
		u, err := url.Parse(host)
		if err != nil {
			return "", err
		}
		if u.Scheme == "http" {
			u.Scheme = "ws"
		}
		if u.Scheme == "https" {
			u.Scheme = "wss"
		}
		if u.Path == "" || u.Path == "/" {
			u.Path = "/websocket/ws"
		}
		q := u.Query()
		if q.Get("session_id") == "" {
			q.Set("session_id", opts.SessionID)
		}
		u.RawQuery = q.Encode()
		return u.String(), nil
	}

	if !strings.Contains(host, ":") && opts.Port > 0 {
		host = host + ":" + strconv.Itoa(opts.Port)
	}
	u := url.URL{
		Scheme: "ws",
		Host:   host,
		Path:   "/websocket/ws",
	}
	q := u.Query()
	q.Set("session_id", opts.SessionID)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func wschannelPayloadThought(payload map[string]any) bool {
	thought, _ := payload[wschannel.PayloadKeyThought].(bool)
	return thought
}
