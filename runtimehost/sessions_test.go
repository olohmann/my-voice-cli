package runtimehost

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	sdk "github.com/github/copilot-sdk/go"
)

type fakeCleaner struct {
	sessions []sdk.SessionMetadata
	deleted  []string
}

func (f *fakeCleaner) ListSessions(context.Context, *sdk.SessionListFilter) ([]sdk.SessionMetadata, error) {
	return f.sessions, nil
}

func (f *fakeCleaner) DeleteSession(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func TestSessionStoreCleansOnlyStaleOwnedSessions(t *testing.T) {
	store := &SessionStore{dir: t.TempDir()}
	staleID := SessionID("stale")
	freshID := SessionID("fresh")
	if err := store.Mark(staleID); err != nil {
		t.Fatal(err)
	}
	if err := store.Mark(freshID); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(store.markerPath(staleID), time.Now().Add(-time.Hour), time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.dir, "other-app-session"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	cleaner := &fakeCleaner{sessions: []sdk.SessionMetadata{
		{SessionID: staleID},
		{SessionID: "other-app-session"},
	}}
	if err := store.CleanupStale(context.Background(), cleaner); err != nil {
		t.Fatal(err)
	}
	if len(cleaner.deleted) != 1 || cleaner.deleted[0] != staleID {
		t.Fatalf("deleted = %v, want [%s]", cleaner.deleted, staleID)
	}
	if _, err := os.Stat(store.markerPath(staleID)); !os.IsNotExist(err) {
		t.Fatalf("stale marker still exists: %v", err)
	}
	if _, err := os.Stat(store.markerPath(freshID)); err != nil {
		t.Fatalf("fresh marker was removed: %v", err)
	}
}

func TestSessionStoreRejectsUnownedSession(t *testing.T) {
	store := &SessionStore{dir: t.TempDir()}
	if err := store.Mark("other-session"); err == nil {
		t.Fatal("expected unowned session error")
	}
}

func TestSessionStoreCleansFreshMarkerFromDeadOwner(t *testing.T) {
	store := &SessionStore{dir: t.TempDir()}
	sessionID := SessionID("crashed")
	marker := []byte(`{"pid":99999999,"created_at":"2026-01-01T00:00:00Z"}`)
	if err := os.WriteFile(store.markerPath(sessionID), marker, 0o600); err != nil {
		t.Fatal(err)
	}

	cleaner := &fakeCleaner{sessions: []sdk.SessionMetadata{{SessionID: sessionID}}}
	if err := store.CleanupStale(context.Background(), cleaner); err != nil {
		t.Fatal(err)
	}
	if len(cleaner.deleted) != 1 || cleaner.deleted[0] != sessionID {
		t.Fatalf("deleted = %v, want crashed session", cleaner.deleted)
	}
}

func TestWriteStateUsesPrivatePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), stateFileName)
	state := State{
		SupervisorPID:   123,
		URL:             "127.0.0.1:4321",
		ConnectionToken: "secret",
	}
	if err := writeState(path, state); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state permissions = %o, want 600", info.Mode().Perm())
	}
}
