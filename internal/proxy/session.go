package proxy

import (
	"context"
	"fmt"
	"sync"

	"github.com/innomon/adk2goose/internal/gooseclient"
)

// SessionManager maintains bidirectional mappings between ADK session IDs
// and Goose session IDs, creating Goose sessions on demand.
type SessionManager struct {
	mu         sync.RWMutex
	adkToGoose map[string]string // adkSessionID → gooseSessionID
	gooseToADK map[string]string // reverse mapping
	client     *gooseclient.Client
	workingDir string
}

// NewSessionManager creates a SessionManager that uses client to start/stop
// Goose agent sessions rooted at workingDir.
func NewSessionManager(client *gooseclient.Client, workingDir string) *SessionManager {
	return &SessionManager{
		adkToGoose: make(map[string]string),
		gooseToADK: make(map[string]string),
		client:     client,
		workingDir: workingDir,
	}
}

// GetOrCreate returns the Goose session ID mapped to adkSessionID, starting a
// new Goose agent session if one does not already exist.
func (sm *SessionManager) GetOrCreate(ctx context.Context, adkSessionID string) (string, error) {
	sm.mu.RLock()
	if gooseID, ok := sm.adkToGoose[adkSessionID]; ok {
		sm.mu.RUnlock()
		return gooseID, nil
	}
	sm.mu.RUnlock()

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Double-check after acquiring write lock.
	if gooseID, ok := sm.adkToGoose[adkSessionID]; ok {
		return gooseID, nil
	}

	resp, err := sm.client.StartAgent(ctx, &gooseclient.StartAgentRequest{
		WorkingDir: sm.workingDir,
	})
	if err != nil {
		return "", fmt.Errorf("start goose agent for ADK session %s: %w", adkSessionID, err)
	}

	sm.adkToGoose[adkSessionID] = resp.ID
	sm.gooseToADK[resp.ID] = adkSessionID

	return resp.ID, nil
}

// Stop stops the Goose agent session mapped to adkSessionID and removes the
// bidirectional mapping.
func (sm *SessionManager) Stop(ctx context.Context, adkSessionID string) error {
	sm.mu.Lock()
	gooseID, ok := sm.adkToGoose[adkSessionID]
	if !ok {
		sm.mu.Unlock()
		return fmt.Errorf("no goose session for ADK session %s", adkSessionID)
	}
	delete(sm.adkToGoose, adkSessionID)
	delete(sm.gooseToADK, gooseID)
	sm.mu.Unlock()

	return sm.client.StopAgent(ctx, gooseID)
}

// GetGooseSessionID returns the Goose session ID for the given ADK session ID.
func (sm *SessionManager) GetGooseSessionID(adkSessionID string) (string, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	gooseID, ok := sm.adkToGoose[adkSessionID]
	return gooseID, ok
}

// ListMappedSessions returns a copy of the current ADK-to-Goose session mappings.
func (sm *SessionManager) ListMappedSessions() map[string]string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	out := make(map[string]string, len(sm.adkToGoose))
	for k, v := range sm.adkToGoose {
		out[k] = v
	}
	return out
}
