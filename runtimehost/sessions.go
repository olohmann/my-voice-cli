package runtimehost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	sdk "github.com/github/copilot-sdk/go"
)

const (
	sessionPrefix   = "my-voice-"
	staleSessionAge = 10 * time.Minute
)

// SessionStore tracks sessions that must be permanently deleted.
type SessionStore struct {
	dir string
}

type sessionMarker struct {
	PID       int       `json:"pid"`
	CreatedAt time.Time `json:"created_at"`
}

// SessionCleaner is the SDK surface required for orphan cleanup.
type SessionCleaner interface {
	ListSessions(context.Context, *sdk.SessionListFilter) ([]sdk.SessionMetadata, error)
	DeleteSession(context.Context, string) error
}

// NewSessionStore creates the private pending-cleanup store.
func NewSessionStore() (*SessionStore, error) {
	runtimeDir, err := ensureRuntimeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(runtimeDir, "pending-sessions")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating session cleanup directory: %w", err)
	}
	return &SessionStore{dir: dir}, nil
}

// SessionID returns a new application-owned session identifier.
func SessionID(id string) string {
	return sessionPrefix + id
}

// Mark records a session before it is created.
func (s *SessionStore) Mark(sessionID string) error {
	if !isOwnedSession(sessionID) {
		return fmt.Errorf("refusing to track unowned session %q", sessionID)
	}
	file, err := os.OpenFile(s.markerPath(sessionID), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("creating session cleanup marker: %w", err)
	}
	marker := sessionMarker{PID: os.Getpid(), CreatedAt: time.Now().UTC()}
	if err := json.NewEncoder(file).Encode(marker); err != nil {
		file.Close()
		_ = os.Remove(s.markerPath(sessionID))
		return fmt.Errorf("writing session cleanup marker: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("closing session cleanup marker: %w", err)
	}
	return nil
}

// Complete removes a marker after permanent deletion succeeds.
func (s *SessionStore) Complete(sessionID string) error {
	err := os.Remove(s.markerPath(sessionID))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("removing session cleanup marker: %w", err)
	}
	return nil
}

// CleanupStale permanently deletes application-owned orphaned sessions.
func (s *SessionStore) CleanupStale(ctx context.Context, client SessionCleaner) error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return fmt.Errorf("reading session cleanup markers: %w", err)
	}

	cutoff := time.Now().Add(-staleSessionAge)
	var candidates []string
	var cleanupErrors []error
	for _, entry := range entries {
		if entry.IsDir() || !isOwnedSession(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("reading marker %s: %w", entry.Name(), err))
			continue
		}
		if info.ModTime().After(cutoff) {
			data, readErr := os.ReadFile(s.markerPath(entry.Name()))
			var marker sessionMarker
			if readErr == nil && json.Unmarshal(data, &marker) == nil && marker.PID > 0 && processAlive(marker.PID) == nil {
				continue
			}
		}
		candidates = append(candidates, entry.Name())
	}
	if len(candidates) == 0 {
		return errors.Join(cleanupErrors...)
	}

	sessions, err := client.ListSessions(ctx, nil)
	if err != nil {
		return fmt.Errorf("listing sessions for orphan cleanup: %w", err)
	}
	existing := make(map[string]bool, len(sessions))
	for _, session := range sessions {
		existing[session.SessionID] = true
	}

	for _, sessionID := range candidates {
		if existing[sessionID] {
			if err := client.DeleteSession(ctx, sessionID); err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("deleting orphaned session %s: %w", sessionID, err))
				continue
			}
		}
		if err := s.Complete(sessionID); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	return errors.Join(cleanupErrors...)
}

func (s *SessionStore) markerPath(sessionID string) string {
	return filepath.Join(s.dir, filepath.Base(sessionID))
}

func isOwnedSession(sessionID string) bool {
	return strings.HasPrefix(sessionID, sessionPrefix) &&
		sessionID == filepath.Base(sessionID) &&
		len(sessionID) > len(sessionPrefix)
}
