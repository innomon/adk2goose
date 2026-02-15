package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/innomon/adk2goose/internal/gooseclient"
	"google.golang.org/genai"
)

func newMockGooseServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("POST /agent/start", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"id":          "goose-session-1",
			"name":        "test",
			"working_dir": "/tmp",
		})
	})

	mux.HandleFunc("POST /agent/stop", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "{}")
	})

	mux.HandleFunc("POST /reply", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		fmt.Fprint(w, `data: {"type":"Message","message":{"role":"assistant","created":1234567890,"content":[{"type":"text","text":"Hello from Goose!"}]},"token_state":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}`+"\n\n")
		flusher.Flush()

		fmt.Fprint(w, `data: {"type":"Finish","reason":"stop","token_state":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}`+"\n\n")
		flusher.Flush()
	})

	mux.HandleFunc("GET /sessions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"sessions": []any{}})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func setupProxy(t *testing.T) (*httptest.Server, *httptest.Server) {
	t.Helper()

	gooseSrv := newMockGooseServer(t)
	client := gooseclient.New(gooseSrv.URL, "")
	sessions := NewSessionManager(client, "/tmp")
	handler := NewHandler(sessions, client)

	proxySrv := httptest.NewServer(handler)
	t.Cleanup(proxySrv.Close)

	return gooseSrv, proxySrv
}

func TestCreateSession(t *testing.T) {
	_, proxySrv := setupProxy(t)

	resp, err := http.Post(proxySrv.URL+"/apps/myapp/users/user1/sessions", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("POST create session: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	id, _ := result["id"].(string)
	if id == "" {
		t.Fatal("expected non-empty id")
	}
	if appName, _ := result["appName"].(string); appName != "myapp" {
		t.Fatalf("expected appName=myapp, got %q", appName)
	}
	if userId, _ := result["userId"].(string); userId != "user1" {
		t.Fatalf("expected userId=user1, got %q", userId)
	}
}

func TestRunSSE_SimpleText(t *testing.T) {
	_, proxySrv := setupProxy(t)

	// Create a session first.
	createResp, err := http.Post(proxySrv.URL+"/apps/myapp/users/user1/sessions", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("POST create session: %v", err)
	}
	defer createResp.Body.Close()

	var createResult map[string]any
	if err := json.NewDecoder(createResp.Body).Decode(&createResult); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	sessionID, _ := createResult["id"].(string)
	if sessionID == "" {
		t.Fatal("expected non-empty session id")
	}

	// Send run_sse request.
	reqBody := map[string]any{
		"new_message": &genai.Content{
			Parts: []*genai.Part{genai.NewPartFromText("hello")},
			Role:  "user",
		},
	}
	reqBytes, _ := json.Marshal(reqBody)

	sseResp, err := http.Post(
		fmt.Sprintf("%s/apps/myapp/users/user1/sessions/%s/run_sse", proxySrv.URL, sessionID),
		"application/json",
		bytes.NewReader(reqBytes),
	)
	if err != nil {
		t.Fatalf("POST run_sse: %v", err)
	}
	defer sseResp.Body.Close()

	if sseResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(sseResp.Body)
		t.Fatalf("expected status 200, got %d: %s", sseResp.StatusCode, body)
	}

	ct := sseResp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("expected Content-Type text/event-stream, got %q", ct)
	}

	// Read SSE events.
	var events []map[string]any
	scanner := bufio.NewScanner(sseResp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		var evt map[string]any
		if err := json.Unmarshal([]byte(payload), &evt); err != nil {
			t.Fatalf("unmarshal SSE event: %v", err)
		}
		events = append(events, evt)
	}

	if len(events) < 2 {
		t.Fatalf("expected at least 2 SSE events, got %d", len(events))
	}

	// First event should contain the message text.
	content, _ := events[0]["content"].(map[string]any)
	if content == nil {
		t.Fatal("expected content in first event")
	}
	parts, _ := content["parts"].([]any)
	foundText := false
	for _, p := range parts {
		pm, _ := p.(map[string]any)
		if text, _ := pm["text"].(string); strings.Contains(text, "Hello from Goose!") {
			foundText = true
			break
		}
	}
	if !foundText {
		t.Fatalf("expected message containing 'Hello from Goose!' in first event, got %+v", events[0])
	}

	// Second event should have turnComplete=true.
	turnComplete, _ := events[1]["turnComplete"].(bool)
	if !turnComplete {
		t.Fatalf("expected turnComplete=true in second event, got %+v", events[1])
	}
}

func TestDeleteSession(t *testing.T) {
	_, proxySrv := setupProxy(t)

	// Create a session first.
	createResp, err := http.Post(proxySrv.URL+"/apps/myapp/users/user1/sessions", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("POST create session: %v", err)
	}
	defer createResp.Body.Close()

	var createResult map[string]any
	if err := json.NewDecoder(createResp.Body).Decode(&createResult); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	sessionID, _ := createResult["id"].(string)

	// Delete the session.
	req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/apps/myapp/users/user1/sessions/%s", proxySrv.URL, sessionID), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE session: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestListSessions(t *testing.T) {
	_, proxySrv := setupProxy(t)

	resp, err := http.Get(proxySrv.URL + "/apps/myapp/users/user1/sessions")
	if err != nil {
		t.Fatalf("GET list sessions: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
}
