# ADK2Goose Proxy

A Go proxy that bridges [Google ADK](https://github.com/google/adk-go) agents with a running [Goose](https://github.com/block/goose) server — translating ADK REST API calls into Goose REST API requests and streaming Goose SSE responses back as ADK events.

## Architecture

```
ADK Client ──► adk2goose proxy ──► Goose Server
  (ADK REST)      (translate)       (Goose REST + SSE)
```

The proxy serves the ADK REST API surface (`/apps/{app}/users/{user}/sessions/...`) and transparently forwards requests to a Goose backend, handling:

- **Session lifecycle** — ADK session create/delete maps to Goose agent start/stop
- **Streaming** — ADK `run_sse` streams are backed by Goose `/reply` SSE streams
- **Type translation** — `genai.Content` ↔ Goose `Message`, `FunctionCall` ↔ `ToolRequest`, etc.
- **Token usage** — Goose `TokenState` maps to ADK `UsageMetadata`

## Quick Start

### Prerequisites

- Go 1.22+
- A running [Goose](https://github.com/block/goose) server

### Build & Run

```bash
go build -o adk2goose ./cmd/proxy
./adk2goose
```

The proxy listens on `:8080` by default and forwards to `http://127.0.0.1:3000`.

### Configuration

All configuration is via environment variables:

| Variable | Default | Description |
|---|---|---|
| `GOOSE_BASE_URL` | `http://127.0.0.1:3000` | Goose server base URL |
| `GOOSE_SECRET_KEY` | *(empty)* | Secret key for Goose API authentication (`X-Secret-Key` header) |
| `LISTEN_ADDR` | `:8080` | Address the proxy listens on |
| `WORKING_DIR` | `.` | Default working directory for new Goose sessions |
| `REQUEST_TIMEOUT` | `5m` | Timeout for streaming requests (Go duration format) |

### Example

```bash
# Start Goose server (separate terminal)
goose server --port 3000

# Start the proxy
export GOOSE_BASE_URL=http://127.0.0.1:3000
export GOOSE_SECRET_KEY=your-secret-key
go run ./cmd/proxy

# Create a session
curl -X POST http://localhost:8080/apps/myapp/users/user1/sessions

# Send a message (streaming)
curl -N -X POST http://localhost:8080/apps/myapp/users/user1/sessions/<session-id>/run_sse \
  -H 'Content-Type: application/json' \
  -d '{"new_message": {"parts": [{"text": "Hello!"}], "role": "user"}}'
```

## API Endpoints

The proxy implements the ADK REST API surface:

| Method | Path | Description |
|---|---|---|
| `POST` | `/apps/{app}/users/{user}/sessions` | Create a new session (starts a Goose agent) |
| `GET` | `/apps/{app}/users/{user}/sessions` | List sessions |
| `POST` | `/apps/{app}/users/{user}/sessions/{id}/run_sse` | Send a message and stream the response via SSE |
| `DELETE` | `/apps/{app}/users/{user}/sessions/{id}` | Delete a session (stops the Goose agent) |

### SSE Event Format

The `run_sse` endpoint returns Server-Sent Events. Each event is a JSON object:

```json
data: {"id":"evt_...","time":1234567890,"invocationId":"inv_...","author":"goose","content":{"parts":[{"text":"Hello!"}],"role":"model"},"turnComplete":false}

data: {"id":"evt_...","time":1234567890,"invocationId":"inv_...","author":"goose","turnComplete":true,"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}
```

## Project Structure

```
adk2goose/
├── cmd/proxy/
│   └── main.go                    # CLI entrypoint with graceful shutdown
├── internal/
│   ├── config/
│   │   └── config.go              # Environment variable configuration
│   ├── gooseclient/
│   │   ├── types.go               # Goose API request/response structs
│   │   └── client.go              # Goose HTTP client with SSE streaming
│   ├── translator/
│   │   ├── adk_to_goose.go        # ADK Content/Event → Goose Message
│   │   ├── goose_to_adk.go        # Goose SSE Event → ADK Event
│   │   ├── tools.go               # Tool schema helpers
│   │   └── translator_test.go     # Unit tests
│   └── proxy/
│       ├── handler.go             # ADK REST API HTTP handler
│       ├── handler_test.go        # Integration tests with mock Goose server
│       └── session.go             # ADK ↔ Goose session mapping
├── ADK2GOOSE_SPEC.md
├── IMPLEMENTATION_PLAN.md
└── go.mod
```

## Testing

```bash
go test ./...
```

Tests include:
- **Unit tests** — translator type conversions (text, function calls, tool responses, SSE events)
- **Integration tests** — full proxy flow with a mock Goose server (session create, SSE streaming, session delete)

## Type Mapping Reference

| ADK Type | Goose Type | Direction |
|---|---|---|
| `genai.Content` (role=user) | `GooseMessage` (role=user) | ADK → Goose |
| `genai.Content` (role=model) | `GooseMessage` (role=assistant) | Goose → ADK |
| `genai.Part{Text}` | `MessageContent{type=text}` | Both |
| `genai.FunctionCall` | `MessageContent{type=toolRequest}` | Both |
| `genai.FunctionResponse` | `MessageContent{type=toolResponse}` | Both |
| `genai.Blob` (inline data) | `MessageContent{type=image}` | ADK → Goose |
| Goose `TokenState` | `genai.GenerateContentResponseUsageMetadata` | Goose → ADK |
| Goose SSE `Message` | `ADKEvent` with content | Goose → ADK |
| Goose SSE `Finish` | `ADKEvent` with `turnComplete=true` | Goose → ADK |
| Goose SSE `Error` | `ADKEvent` with error fields | Goose → ADK |

## License

Apache License 2.0 — see [LICENSE](LICENSE).
