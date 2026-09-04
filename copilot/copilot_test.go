package copilot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	sdk "github.com/github/copilot-sdk/go"
	"github.com/olohmann/my-voice-cli/runtimehost"
)

type fakeSessionCleaner struct {
	sessions  []sdk.SessionMetadata
	deleteErr error
	deleted   []string
}

func (f *fakeSessionCleaner) ListSessions(context.Context, *sdk.SessionListFilter) ([]sdk.SessionMetadata, error) {
	return f.sessions, nil
}

func (f *fakeSessionCleaner) DeleteSession(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return f.deleteErr
}

func TestSessionConfigIsMinimalAndIsolated(t *testing.T) {
	cfg := SessionConfig("my-voice-test", "rewrite", GenerateOptions{
		Model:           "gpt-5.6-luna",
		ReasoningEffort: "low",
		Stream:          true,
	})

	if cfg.SessionID != "my-voice-test" {
		t.Fatalf("SessionID = %q", cfg.SessionID)
	}
	if cfg.Model != "gpt-5.6-luna" || cfg.ReasoningEffort != "low" {
		t.Fatalf("unexpected model config: %s/%s", cfg.Model, cfg.ReasoningEffort)
	}
	if cfg.ReasoningSummary != sdk.ReasoningSummaryNone {
		t.Fatalf("ReasoningSummary = %q", cfg.ReasoningSummary)
	}
	if cfg.AvailableTools == nil || len(cfg.AvailableTools) != 0 {
		t.Fatalf("AvailableTools must be a non-nil empty allowlist")
	}
	assertBool(t, "EnableConfigDiscovery", cfg.EnableConfigDiscovery, false)
	assertBool(t, "SkipCustomInstructions", cfg.SkipCustomInstructions, true)
	assertBool(t, "EnableOnDemandInstructionDiscovery", cfg.EnableOnDemandInstructionDiscovery, false)
	assertBool(t, "EnableFileHooks", cfg.EnableFileHooks, false)
	assertBool(t, "EnableHostGitOperations", cfg.EnableHostGitOperations, false)
	assertBool(t, "EnableSessionStore", cfg.EnableSessionStore, false)
	assertBool(t, "EnableSkills", cfg.EnableSkills, false)
	assertBool(t, "EnableSessionTelemetry", cfg.EnableSessionTelemetry, false)
	assertBool(t, "SkipEmbeddingRetrieval", cfg.SkipEmbeddingRetrieval, true)
	assertBool(t, "Streaming", cfg.Streaming, true)
	if cfg.Memory == nil || cfg.Memory.Enabled {
		t.Fatal("memory must be disabled")
	}
	if cfg.InfiniteSessions == nil {
		t.Fatal("infinite sessions config is required")
	}
	assertBool(t, "InfiniteSessions.Enabled", cfg.InfiniteSessions.Enabled, false)
}

func TestDeleteSessionRemovesCleanupMarker(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	store, err := runtimehost.NewSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	sessionID := runtimehost.SessionID("success")
	if err := store.Mark(sessionID); err != nil {
		t.Fatal(err)
	}

	cleaner := &fakeSessionCleaner{}
	if err := deleteSession(cleaner, store, sessionID); err != nil {
		t.Fatal(err)
	}
	if len(cleaner.deleted) != 1 || cleaner.deleted[0] != sessionID {
		t.Fatalf("deleted = %v", cleaner.deleted)
	}

	runtimeDir, err := runtimehost.RuntimeDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(runtimeDir, "pending-sessions", sessionID)); !os.IsNotExist(err) {
		t.Fatalf("cleanup marker still exists: %v", err)
	}
}

func TestDeleteSessionRetainsMarkerWhenSessionStillExists(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	store, err := runtimehost.NewSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	sessionID := runtimehost.SessionID("failure")
	if err := store.Mark(sessionID); err != nil {
		t.Fatal(err)
	}

	cleaner := &fakeSessionCleaner{
		sessions:  []sdk.SessionMetadata{{SessionID: sessionID}},
		deleteErr: errors.New("delete failed"),
	}
	if err := deleteSession(cleaner, store, sessionID); err == nil {
		t.Fatal("expected permanent deletion error")
	}

	runtimeDir, err := runtimehost.RuntimeDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(runtimeDir, "pending-sessions", sessionID)); err != nil {
		t.Fatalf("cleanup marker was not retained: %v", err)
	}
}

func assertBool(t *testing.T, name string, value *bool, want bool) {
	t.Helper()
	if value == nil || *value != want {
		t.Fatalf("%s = %v, want %v", name, value, want)
	}
}
