package tunnel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/eslusarenko/port-client/internal/protocol"
)

// PrintConfig controls request/header logging to stdout.
type PrintConfig struct {
	Requests     bool
	Headers      bool   // print all headers (--all-request-headers)
	HeaderFilter string // print only these headers, comma-separated (--request-headers)
}

// headerFilter parses HeaderFilter into a lowercase list of names.
// Returns nil when all headers should be printed.
func (p PrintConfig) resolveHeaderFilter() []string {
	if p.HeaderFilter == "" {
		return nil // nil = all
	}
	// Try JSON array: ["host","user-agent"]
	var list []string
	if err := json.Unmarshal([]byte(p.HeaderFilter), &list); err == nil {
		out := make([]string, 0, len(list))
		for _, h := range list {
			if t := strings.ToLower(strings.TrimSpace(h)); t != "" {
				out = append(out, t)
			}
		}
		return out
	}
	// Comma-separated: host,user-agent
	parts := strings.Split(p.HeaderFilter, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.ToLower(strings.TrimSpace(p)); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// wantHeaders reports whether any header logging is requested.
func (p PrintConfig) wantHeaders() bool { return p.Headers || p.HeaderFilter != "" }

// Client manages a tunnel connection to the server and forwards incoming
// HTTP requests to the local target service.
type Client struct {
	serverAddr  string
	targetURL   *url.URL
	logger      *slog.Logger
	maxBody     int64
	httpClient  *http.Client
	print       PrintConfig
	headerNames []string // nil = all; populated when --header filter is set
	setHost     string   // override Host sent to local app (--set-host)
	domain      string   // requested subdomain (--domain)
	apiKey      string

	conn    *websocket.Conn
	writeMu sync.Mutex
	done    chan struct{}
}

// New creates a tunnel client.
func New(serverAddr string, targetURL *url.URL, logger *slog.Logger, maxBody int64, print PrintConfig, setHost, domain, apiKey string) *Client {
	return &Client{
		serverAddr:  serverAddr,
		targetURL:   targetURL,
		logger:      logger,
		maxBody:     maxBody,
		print:       print,
		headerNames: print.resolveHeaderFilter(),
		setHost:     setHost,
		domain:      domain,
		apiKey:      apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				// Don't follow redirects — pass them through to the end user.
				return http.ErrUseLastResponse
			},
		},
		done: make(chan struct{}),
	}
}

// Connect dials the server, waits for TunnelReady, starts the read loop
// in a background goroutine, and returns the public URL.
// The read loop runs until ctx is cancelled or the connection drops.
// Call Wait() to block until the tunnel is fully closed.
func (c *Client) Connect(ctx context.Context) (string, error) {
	wsURL := c.serverAddr + "/tunnel/connect"
	if c.domain != "" {
		wsURL += "?subdomain=" + url.QueryEscape(c.domain)
	}
	var dialOptions *websocket.DialOptions
	if c.apiKey != "" {
		dialOptions = &websocket.DialOptions{
			HTTPHeader: http.Header{"Authorization": []string{"Bearer " + c.apiKey}},
		}
	}
	conn, resp, err := websocket.Dial(ctx, wsURL, dialOptions)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			if c.apiKey != "" {
				return "", fmt.Errorf("server rejected API key (invalid or revoked)")
			}
			return "", fmt.Errorf("server requires authentication: set PORT_API_KEY or add it to ~/.port.conf")
		}
		return "", fmt.Errorf("dial server: %w", err)
	}
	conn.SetReadLimit(c.maxBody + protocol.HeaderSize + 4 + 4096)
	c.conn = conn

	// Read TunnelReady.
	_, data, err := conn.Read(ctx)
	if err != nil {
		_ = conn.CloseNow()
		return "", fmt.Errorf("read tunnel ready: %w", err)
	}

	msgType, _, payload, err := protocol.DecodeMessage(data)
	if err != nil {
		_ = conn.CloseNow()
		return "", fmt.Errorf("decode tunnel ready: %w", err)
	}

	if msgType == protocol.TypeTunnelError {
		var tunnelErr protocol.TunnelError
		_ = json.Unmarshal(payload, &tunnelErr)
		_ = conn.CloseNow()
		return "", fmt.Errorf("server error: %s", tunnelErr.Error)
	}

	if msgType != protocol.TypeTunnelReady {
		_ = conn.CloseNow()
		return "", fmt.Errorf("unexpected message type: %d", msgType)
	}

	var ready protocol.TunnelReady
	if err := json.Unmarshal(payload, &ready); err != nil {
		_ = conn.CloseNow()
		return "", fmt.Errorf("unmarshal tunnel ready: %w", err)
	}

	// Start background loops.
	go c.pingLoop(ctx)
	go func() {
		c.readLoop(ctx)
		close(c.done)
	}()

	return ready.URL, nil
}

// Wait blocks until the tunnel read loop exits.
func (c *Client) Wait() {
	<-c.done
}

func (c *Client) pingLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		case <-ticker.C:
			msg := protocol.EncodeMessage(protocol.TypePing, 0, nil)
			c.writeMu.Lock()
			err := c.conn.Write(ctx, websocket.MessageBinary, msg)
			c.writeMu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

func (c *Client) readLoop(ctx context.Context) {
	defer func() { _ = c.conn.CloseNow() }()

	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			return
		}

		msgType, requestID, payload, err := protocol.DecodeMessage(data)
		if err != nil {
			c.logger.Warn("malformed message from server", "error", err)
			continue
		}

		switch msgType {
		case protocol.TypeHttpRequest:
			go c.handleRequest(ctx, requestID, payload)

		case protocol.TypePong:
			// Expected response to our pings.

		case protocol.TypeShutdown:
			c.logger.Info("server initiated shutdown")
			return

		default:
			c.logger.Warn("unknown message type from server", "type", msgType)
		}
	}
}

// logRequest prints request line and/or headers to stdout according to PrintConfig.
// statusCode 0 means the request failed before a response was received.
func (c *Client) logRequest(meta protocol.HttpRequestMeta, statusCode int, forwardErr error) {
	if !c.print.Requests && !c.print.wantHeaders() {
		return
	}
	var sb strings.Builder
	if c.print.Requests {
		if statusCode > 0 {
			_, _ = fmt.Fprintf(&sb, "%s %s → %d\n", meta.Method, meta.Path, statusCode)
		} else {
			_, _ = fmt.Fprintf(&sb, "%s %s → ERR: %v\n", meta.Method, meta.Path, forwardErr)
		}
	}
	if c.print.wantHeaders() {
		if c.headerNames == nil {
			// --all-request-headers: print all, Host first.
			if meta.Host != "" {
				_, _ = fmt.Fprintf(&sb, "Host: %s\n", meta.Host)
			}
			for k, vs := range meta.Headers {
				for _, v := range vs {
					_, _ = fmt.Fprintf(&sb, "%s: %s\n", k, v)
				}
			}
		} else {
			// --request-headers list: only requested headers, in specified order.
			for _, name := range c.headerNames {
				if name == "host" {
					if meta.Host != "" {
						_, _ = fmt.Fprintf(&sb, "Host: %s\n", meta.Host)
					}
					continue
				}
				for k, vs := range meta.Headers {
					if strings.ToLower(k) == name {
						for _, v := range vs {
							_, _ = fmt.Fprintf(&sb, "%s: %s\n", k, v)
						}
					}
				}
			}
		}
	}
	_, _ = fmt.Fprint(os.Stdout, sb.String()+"\n")
}

func (c *Client) handleRequest(ctx context.Context, requestID uint32, payload []byte) {
	meta, body, err := protocol.DecodeHttpRequestMeta(payload)
	if err != nil {
		c.logger.Error("decode http request", "error", err)
		c.sendRequestError(ctx, requestID, err.Error())
		return
	}

	// Build the request to the local service.
	targetURL := *c.targetURL
	parsed, parseErr := url.Parse(meta.Path)
	if parseErr == nil {
		targetURL.Path = parsed.Path
		targetURL.RawQuery = parsed.RawQuery
	} else {
		targetURL.Path = meta.Path
	}

	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, meta.Method, targetURL.String(), bodyReader)
	if err != nil {
		c.sendRequestError(ctx, requestID, err.Error())
		return
	}

	for k, vs := range meta.Headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	// Set the Host header forwarded to the local service.
	// --set-host overrides; otherwise forward the original public hostname.
	switch {
	case c.setHost != "":
		req.Host = c.setHost
	case meta.Host != "":
		req.Host = meta.Host
	}
	req.ContentLength = meta.ContentLength

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logRequest(meta, 0, err)
		c.sendRequestError(ctx, requestID, err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBody))
	if err != nil {
		c.logRequest(meta, 0, err)
		c.sendRequestError(ctx, requestID, err.Error())
		return
	}

	c.logRequest(meta, resp.StatusCode, nil)

	respMeta := protocol.HttpResponseMeta{
		StatusCode:    resp.StatusCode,
		Headers:       resp.Header,
		ContentLength: int64(len(respBody)),
	}

	respPayload, err := protocol.EncodeHttpMeta(respMeta, respBody)
	if err != nil {
		c.sendRequestError(ctx, requestID, err.Error())
		return
	}

	msg := protocol.EncodeMessage(protocol.TypeHttpResponse, requestID, respPayload)

	c.writeMu.Lock()
	_ = c.conn.Write(ctx, websocket.MessageBinary, msg)
	c.writeMu.Unlock()
}

func (c *Client) sendRequestError(ctx context.Context, requestID uint32, errMsg string) {
	reqErr := protocol.RequestError{RequestID: requestID, Error: errMsg}
	payload, _ := json.Marshal(reqErr)
	msg := protocol.EncodeMessage(protocol.TypeRequestError, 0, payload)

	c.writeMu.Lock()
	_ = c.conn.Write(ctx, websocket.MessageBinary, msg)
	c.writeMu.Unlock()
}
