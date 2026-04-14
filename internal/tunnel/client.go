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
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/eslusarenko/port-client/internal/protocol"
)

// Client manages a tunnel connection to the server and forwards incoming
// HTTP requests to the local target service.
type Client struct {
	serverAddr string
	targetURL  *url.URL
	logger     *slog.Logger
	maxBody    int64
	httpClient *http.Client

	conn    *websocket.Conn
	writeMu sync.Mutex
	done    chan struct{}
}

// New creates a tunnel client.
func New(serverAddr string, targetURL *url.URL, logger *slog.Logger, maxBody int64) *Client {
	return &Client{
		serverAddr: serverAddr,
		targetURL:  targetURL,
		logger:     logger,
		maxBody:    maxBody,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				// Don't follow redirects — pass them through to the end user.
				return http.ErrUseLastResponse
			},
		},
		done:       make(chan struct{}),
	}
}

// Connect dials the server, waits for TunnelReady, starts the read loop
// in a background goroutine, and returns the public URL.
// The read loop runs until ctx is cancelled or the connection drops.
// Call Wait() to block until the tunnel is fully closed.
func (c *Client) Connect(ctx context.Context) (string, error) {
	wsURL := c.serverAddr + "/tunnel/connect"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
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
	// Forward the original public Host header so the local service sees the
	// tunnel hostname (e.g. abc123.tnls.lt) instead of localhost:port.
	if meta.Host != "" {
		req.Host = meta.Host
	}
	req.ContentLength = meta.ContentLength

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.sendRequestError(ctx, requestID, err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBody))
	if err != nil {
		c.sendRequestError(ctx, requestID, err.Error())
		return
	}

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
