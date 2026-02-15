# ADK2GOOSE Implementation Plan

## Goal

Build a Go proxy that allows an ADK (`google/adk-go`) agent to use a running Goose (`block/goose`) server as its backend — translating ADK invocations into Goose REST API calls and streaming Goose SSE responses back as ADK events.

---

## Phase 0: Project Bootstrap

- [x] `go mod init github.com/innomon/adk2goose`
- [x] Add dependencies:
  - `google.golang.org/genai` (ADK genai types: Content, Part, FunctionCall)
  - `net/http` stdlib (Goose REST client)
  - `bufio.Scanner` stdlib for SSE parsing (no external SSE library needed)
- [x] Create directory layout:

```
adk2goose/
├── cmd/
│   └── proxy/
│       └── main.go              # Entrypoint
├── internal/
│   ├── gooseclient/
│   │   ├── client.go            # Goose HTTP client
│   │   ├── types.go             # Goose JSON request/response structs
│   │   └── sse.go               # SSE stream parser
│   ├── translator/
│   │   ├── adk_to_goose.go      # ADK Event/Content → Goose Message
│   │   ├── goose_to_adk.go      # Goose MessageEvent → ADK Event
│   │   └── tools.go             # Tool schema mapping
│   ├── proxy/
│   │   ├── handler.go           # Main proxy HTTP handler (serves ADK REST)
│   │   └── session.go           # Session lifecycle (start/stop/resume)
│   └── config/
│       └── config.go            # Configuration (env vars, flags)
├── ADK2GOOSE_SPEC.md
├── IMPLEMENTATION_PLAN.md
└── go.mod
```

---

## Phase 1: Goose Client & Data Types

### 1.1 — Define Goose JSON Structs (`internal/gooseclient/types.go`)

Map the Goose server's JSON schema to Go structs:

| Goose Type       | Go Struct                  | Notes                                |
|------------------|----------------------------|--------------------------------------|
| `Message`        | `GooseMessage`             | `id`, `role`, `content[]`, `timestamp` |
| `MessageContent` | `GooseContent` (union)     | Text, ToolRequest, ToolResponse, Thinking |
| `TextContent`    | `GooseTextContent`         | `{ "text": "..." }`                 |
| `ToolCall`       | `GooseToolCall`            | `{ "id", "tool_call": {...} }`      |
| `Session`        | `GooseSession`             | `id`, `name`, `working_dir`, etc.   |
| `TokenState`     | `GooseTokenState`          | Input/output/total token counters    |
| `MessageEvent`   | `GooseSSEEvent` (union)    | Message, Error, Finish, Ping, etc.  |
| `ReplyRequest`   | `GooseReplyRequest`        | `user_message`, `session_id`, `conversation_so_far` |
| `StartAgentReq`  | `GooseStartAgentRequest`   | `working_dir`, `recipe?`            |

### 1.2 — Goose HTTP Client (`internal/gooseclient/client.go`)

```go
type Client struct {
    BaseURL   string
    SecretKey string
    HTTP      *http.Client
}

// Core methods:
func (c *Client) StartAgent(ctx context.Context, req *StartAgentRequest) (*GooseSession, error)
func (c *Client) StopAgent(ctx context.Context, sessionID string) error
func (c *Client) ResumeAgent(ctx context.Context, sessionID string) (*GooseSession, error)
func (c *Client) Reply(ctx context.Context, req *ReplyRequest) (<-chan SSEEvent, error)  // streaming
func (c *Client) GetSession(ctx context.Context, sessionID string) (*GooseSession, error)
func (c *Client) ListSessions(ctx context.Context) ([]GooseSession, error)
func (c *Client) ConfirmTool(ctx context.Context, req *ToolConfirmationRequest) error
```

- All requests include header `X-Secret-Key: <secret>`
- `Reply()` returns a channel of parsed SSE events; internally reads `text/event-stream` with `bufio.Scanner`
- Use `context.Context` for cancellation and timeouts

### 1.3 — SSE Stream Parser (`internal/gooseclient/sse.go`)

- Parse `data: <json>\n\n` lines from the `/reply` response body
- Deserialize into `GooseSSEEvent` discriminated union (check for `message`, `error`, `finish` keys)
- Emit parsed events on a `chan SSEEvent`; close channel on stream end or context cancellation
- Handle reconnection / keep-alive `Ping` events

---

## Phase 2: ADK ↔ Goose Translation Layer

### 2.1 — ADK → Goose (`internal/translator/adk_to_goose.go`)

| ADK Source                      | Goose Target              | Logic                                            |
|---------------------------------|---------------------------|--------------------------------------------------|
| `*genai.Content` (user role)    | `GooseMessage` (user)     | Extract text parts → `TextContent`               |
| `session.Event` (tool response) | `GooseMessage` (user)     | Wrap tool result as `ToolResponse` content block  |
| ADK `RunConfig` (model, params) | `/agent/update_provider`  | Map model name + generation config               |

Key function:

```go
func ADKContentToGooseMessage(content *genai.Content) *GooseMessage
func ADKEventToGooseMessage(event *session.Event) *GooseMessage
```

### 2.2 — Goose → ADK (`internal/translator/goose_to_adk.go`)

| Goose Source                     | ADK Target             | Logic                                          |
|----------------------------------|------------------------|-------------------------------------------------|
| SSE `Message` (assistant role)   | `session.Event`        | Map text → `genai.Content`, tool calls → `FunctionCall` |
| SSE `Finish`                     | Final `session.Event`  | Mark `TurnComplete`, attach token usage as `UsageMetadata` |
| SSE `Error`                      | `error` return         | Wrap in ADK-compatible error                    |
| `GooseToolCall`                  | `genai.FunctionCall`   | Map tool name + args JSON                       |

Key function:

```go
func GooseSSEEventToADKEvent(sse *SSEEvent, invocationID, branch string) (*session.Event, error)
func GooseTokenStateToUsageMetadata(ts *GooseTokenState) *genai.GenerateContentResponseUsageMetadata
```

### 2.3 — Tool Schema Translation (`internal/translator/tools.go`)

- ADK `tool.Tool` → Goose extension tool definition (for informational purposes / logging)
- Goose tool confirmations → ADK `ToolConfirmation` flow
- This is secondary — Goose manages its own tools via extensions; the proxy primarily forwards messages

---

## Phase 3: Session Manager

### 3.1 — Session Mapping (`internal/proxy/session.go`)

Maintain a bidirectional mapping between ADK sessions and Goose sessions:

```go
type SessionManager struct {
    mu       sync.RWMutex
    adkToGoose map[string]string  // adkSessionID → gooseSessionID
    gooseToADK map[string]string  // reverse
    client     *gooseclient.Client
}

func (sm *SessionManager) GetOrCreate(ctx context.Context, adkSessionID, workingDir string) (string, error)
func (sm *SessionManager) Stop(ctx context.Context, adkSessionID string) error
```

- On first ADK request for a session, call `POST /agent/start` to create a Goose session
- Cache the mapping for subsequent requests
- On ADK session delete, call `POST /agent/stop`

---

## Phase 4: Proxy HTTP Handler (ADK REST Interface)

### 4.1 — Serve ADK REST Endpoints (`internal/proxy/handler.go`)

Implement the ADK REST API surface that `adkrest` expects:

| ADK Endpoint                                              | Proxy Action                                       |
|-----------------------------------------------------------|----------------------------------------------------|
| `POST /apps/{app}/users/{user}/sessions`                  | Create Goose session via `/agent/start`            |
| `GET  /apps/{app}/users/{user}/sessions`                  | List mapped Goose sessions via `/sessions`         |
| `POST /apps/{app}/users/{user}/sessions/{sid}/run_sse`    | Translate → `POST /reply`, stream SSE back         |
| `GET  /apps/{app}/users/{user}/sessions/{sid}/artifacts/*`| Proxy to Goose session export or extension resources|

The critical path is `run_sse`:

```
ADK Client → POST /run_sse (genai.Content)
  → Translator: ADK Content → Goose Message
  → Goose Client: POST /reply (SSE stream)
  → For each SSE event:
      → Translator: Goose Event → ADK Event
      → Write SSE to ADK client response
```

### 4.2 — Streaming Pipeline

```go
func (h *Handler) handleRunSSE(w http.ResponseWriter, r *http.Request) {
    // 1. Parse ADK request body (user content, session ID)
    // 2. Resolve Goose session ID via SessionManager
    // 3. Translate ADK content → Goose message
    // 4. Call gooseclient.Reply() → chan SSEEvent
    // 5. Set response headers: Content-Type: text/event-stream
    // 6. For each event from channel:
    //      a. Translate to ADK Event
    //      b. JSON-encode and write as SSE data line
    //      c. Flush
    // 7. Handle context cancellation (client disconnect)
}
```

---

## Phase 5: Configuration & Auth

### 5.1 — Config (`internal/config/config.go`)

```go
type Config struct {
    GooseBaseURL  string  // env: GOOSE_BASE_URL (default: http://127.0.0.1:3000)
    GooseSecret   string  // env: GOOSE_SECRET_KEY
    ListenAddr    string  // env: LISTEN_ADDR (default: :8080)
    WorkingDir    string  // env: WORKING_DIR (default: ".")
    RequestTimeout time.Duration // env: REQUEST_TIMEOUT (default: 5m)
}
```

### 5.2 — Auth Handler

- Outbound: Attach `X-Secret-Key` header to all Goose requests
- Inbound: Optionally validate ADK client requests (API key / bearer token) — configurable

---

## Phase 6: Error Handling & Resilience

| Goose Status | Proxy Behavior                                                  |
|--------------|-----------------------------------------------------------------|
| 401          | Return ADK error: "Goose authentication failed"                |
| 404          | Session not found — clear mapping, return ADK error             |
| 412 / 424    | Agent not ready — retry once after 1s, then error               |
| 429          | Exponential backoff: 1s → 2s → 4s → 8s (max 3 retries)        |
| 500          | Return ADK internal error, log details                          |
| 503          | Goose unavailable — return ADK service unavailable              |
| SSE disconnect | Close ADK SSE stream, return partial results if any           |

---

## Phase 7: Integration Testing

### 7.1 — Mock Goose Server

Build a lightweight `httptest.Server` that simulates:
- `POST /agent/start` → returns a session JSON
- `POST /reply` → streams canned SSE events (message → finish)
- `POST /agent/stop` → returns 200

### 7.2 — Test Cases

| Test                             | Validates                                              |
|----------------------------------|--------------------------------------------------------|
| `TestStartSession`               | ADK session create → Goose `/agent/start` call         |
| `TestRunSSE_SimpleText`          | User text → Goose reply → ADK event stream             |
| `TestRunSSE_ToolCall`            | Goose returns tool call → ADK receives FunctionCall    |
| `TestRunSSE_StreamCancel`        | Client disconnects mid-stream → context cancelled      |
| `TestErrorTranslation_429`       | Goose 429 → retry with backoff → ADK error if exhausted|
| `TestErrorTranslation_500`       | Goose 500 → ADK internal error                        |
| `TestSessionMapping`             | Multiple ADK sessions → distinct Goose sessions        |

### 7.3 — End-to-End Smoke Test

- Start a real Goose server (if available) or the mock
- Start the proxy
- Use ADK's `Runner.Run()` pointed at the proxy
- Send a user message, verify streamed response events

---

## Phase 8: Entrypoint (`cmd/proxy/main.go`)

```go
func main() {
    cfg := config.Load()
    gooseClient := gooseclient.New(cfg.GooseBaseURL, cfg.GooseSecret)
    sessionMgr := proxy.NewSessionManager(gooseClient)
    handler := proxy.NewHandler(sessionMgr, gooseClient)

    srv := &http.Server{
        Addr:    cfg.ListenAddr,
        Handler: handler,
    }

    // Graceful shutdown on SIGINT/SIGTERM
    go func() {
        sigCh := make(chan os.Signal, 1)
        signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
        <-sigCh
        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()
        srv.Shutdown(ctx)
    }()

    log.Printf("adk2goose proxy listening on %s → %s", cfg.ListenAddr, cfg.GooseBaseURL)
    srv.ListenAndServe()
}
```

---

## Execution Order Summary

| Phase | Deliverable                        | Depends On | Status |
|-------|------------------------------------|------------|--------|
| 0     | Project scaffold, `go.mod`         | —          | ✅ Done |
| 1     | Goose client + types + SSE parser  | Phase 0    | ✅ Done |
| 2     | ADK ↔ Goose translators            | Phase 1    | ✅ Done |
| 3     | Session manager                    | Phase 1    | ✅ Done |
| 4     | Proxy HTTP handler                 | Phase 2, 3 | ✅ Done |
| 5     | Config & auth                      | Phase 0    | ✅ Done |
| 6     | Error handling & retry             | Phase 1    | ✅ Done |
| 7     | Tests (mock server + integration)  | Phase 4    | ✅ Done |
| 8     | CLI entrypoint + graceful shutdown | Phase 4, 5 | ✅ Done |

Phases 2, 3, 5, and 6 can be developed in parallel once Phase 1 is complete.
