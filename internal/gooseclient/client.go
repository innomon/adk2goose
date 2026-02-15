package gooseclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client is an HTTP client for the Goose agent API.
type Client struct {
	BaseURL   string
	SecretKey string
	HTTP      *http.Client
}

// New creates a new Goose API client.
func New(baseURL, secretKey string) *Client {
	return &Client{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		SecretKey: secretKey,
		HTTP:      &http.Client{},
	}
}

// doJSON is a helper that sends a JSON request and decodes the JSON response.
func (c *Client) doJSON(ctx context.Context, method, path string, body, result any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.SecretKey != "" {
		req.Header.Set("X-Secret-Key", c.SecretKey)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}

	return nil
}

// StartAgent starts a new Goose agent session.
func (c *Client) StartAgent(ctx context.Context, req *StartAgentRequest) (*StartAgentResponse, error) {
	var resp StartAgentResponse
	if err := c.doJSON(ctx, http.MethodPost, "/agent/start", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// StopAgent stops a running Goose agent session.
func (c *Client) StopAgent(ctx context.Context, sessionID string) error {
	return c.doJSON(ctx, http.MethodPost, "/agent/stop", &StopAgentRequest{SessionID: sessionID}, nil)
}

// ResumeAgent resumes a previously stopped session.
func (c *Client) ResumeAgent(ctx context.Context, req *ResumeAgentRequest) (*StartAgentResponse, error) {
	var resp StartAgentResponse
	if err := c.doJSON(ctx, http.MethodPost, "/agent/resume", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Reply sends a user message and returns a channel of server-sent events.
func (c *Client) Reply(ctx context.Context, req *ReplyRequest) (<-chan SSEEvent, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request body: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/reply", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if c.SecretKey != "" {
		httpReq.Header.Set("X-Secret-Key", c.SecretKey)
	}

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan SSEEvent)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()

			if line == "" || strings.HasPrefix(line, ":") {
				continue
			}

			if strings.HasPrefix(line, "data: ") {
				payload := strings.TrimPrefix(line, "data: ")
				var event SSEEvent
				if err := json.Unmarshal([]byte(payload), &event); err != nil {
					continue
				}
				select {
				case ch <- event:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return ch, nil
}

// GetSession retrieves the full history of a session.
func (c *Client) GetSession(ctx context.Context, sessionID string) (*SessionHistoryResponse, error) {
	var resp SessionHistoryResponse
	if err := c.doJSON(ctx, http.MethodGet, "/sessions/"+sessionID, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListSessions returns all known sessions.
func (c *Client) ListSessions(ctx context.Context) (*SessionListResponse, error) {
	var resp SessionListResponse
	if err := c.doJSON(ctx, http.MethodGet, "/sessions", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
