package proxy

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/innomon/adk2goose/internal/gooseclient"
	"github.com/innomon/adk2goose/internal/translator"
	"google.golang.org/genai"
)

// Handler implements the ADK REST API surface and delegates to Goose via the
// translator and gooseclient packages.
type Handler struct {
	sessions *SessionManager
	client   *gooseclient.Client
	mux      *http.ServeMux
}

// NewHandler creates a Handler that serves the ADK REST API routes.
func NewHandler(sessions *SessionManager, client *gooseclient.Client) *Handler {
	h := &Handler{
		sessions: sessions,
		client:   client,
		mux:      http.NewServeMux(),
	}

	h.mux.HandleFunc("POST /apps/{app}/users/{user}/sessions", h.handleCreateSession)
	h.mux.HandleFunc("GET /apps/{app}/users/{user}/sessions", h.handleListSessions)
	h.mux.HandleFunc("POST /apps/{app}/users/{user}/sessions/{session}/run_sse", h.handleRunSSE)
	h.mux.HandleFunc("DELETE /apps/{app}/users/{user}/sessions/{session}", h.handleDeleteSession)

	return h
}

// ServeHTTP delegates to the internal mux.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

// RunSSERequest is the JSON body sent by the ADK for the run_sse endpoint.
type RunSSERequest struct {
	NewMessage *genai.Content `json:"new_message"`
}

func (h *Handler) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	app := r.PathValue("app")
	user := r.PathValue("user")

	adkSessionID := fmt.Sprintf("%s_%s_%d", app, user, time.Now().UnixNano())

	_, err := h.sessions.GetOrCreate(r.Context(), adkSessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("create session: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":      adkSessionID,
		"appName": app,
		"userId":  user,
		"state":   map[string]any{},
		"events":  []any{},
	})
}

func (h *Handler) handleListSessions(w http.ResponseWriter, r *http.Request) {
	sessions := h.sessions.ListMappedSessions()

	result := make([]map[string]any, 0, len(sessions))
	for adkID := range sessions {
		result = append(result, map[string]any{
			"id":     adkID,
			"state":  map[string]any{},
			"events": []any{},
		})
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) handleRunSSE(w http.ResponseWriter, r *http.Request) {
	adkSessionID := r.PathValue("session")

	var req RunSSERequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode request: %v", err))
		return
	}

	if req.NewMessage == nil {
		writeError(w, http.StatusBadRequest, "new_message is required")
		return
	}

	gooseSessionID, err := h.sessions.GetOrCreate(r.Context(), adkSessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("session lookup: %v", err))
		return
	}

	replyReq := translator.ADKRunSSERequestToReplyRequest(gooseSessionID, req.NewMessage)

	eventCh, err := h.client.Reply(r.Context(), replyReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("goose reply: %v", err))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	invocationID := fmt.Sprintf("inv_%d", time.Now().UnixNano())

	for {
		select {
		case <-r.Context().Done():
			return
		case sse, ok := <-eventCh:
			if !ok {
				return
			}

			adkEvent, err := translator.GooseSSEEventToADKEvent(&sse, invocationID)
			if err != nil {
				log.Printf("translate SSE event: %v", err)
				continue
			}
			if adkEvent == nil {
				continue
			}

			jsonBytes, err := json.Marshal(adkEvent)
			if err != nil {
				log.Printf("marshal ADK event: %v", err)
				continue
			}

			fmt.Fprintf(w, "data: %s\n\n", jsonBytes)
			flusher.Flush()
		}
	}
}

func (h *Handler) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	adkSessionID := r.PathValue("session")

	if err := h.sessions.Stop(r.Context(), adkSessionID); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("stop session: %v", err))
		return
	}

	w.WriteHeader(http.StatusOK)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write JSON response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
