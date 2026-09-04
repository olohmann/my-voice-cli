package runtimehost

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	sdk "github.com/github/copilot-sdk/go"
)

var stdoutStopMu sync.Mutex

const (
	appName        = "my-voice"
	stateFileName  = "runtime.json"
	lockFileName   = "runtime.lock"
	runtimeLogName = "runtime.log"
	startTimeout   = 20 * time.Second
	stopTimeout    = 10 * time.Second
)

// State describes a managed Copilot runtime.
type State struct {
	SupervisorPID   int       `json:"supervisor_pid"`
	URL             string    `json:"url"`
	ConnectionToken string    `json:"connection_token"`
	Version         string    `json:"version"`
	ProtocolVersion int       `json:"protocol_version"`
	StartedAt       time.Time `json:"started_at"`
}

// RuntimeDir returns the private directory used for runtime coordination.
func RuntimeDir() (string, error) {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, appName), nil
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolving user cache directory: %w", err)
	}
	return filepath.Join(cacheDir, appName, "runtime"), nil
}

// DirectClientOptions returns options for an SDK-owned one-shot runtime.
func DirectClientOptions() *sdk.ClientOptions {
	return &sdk.ClientOptions{
		BaseDirectory:             copilotHome(),
		LogLevel:                  "error",
		Mode:                      sdk.ModeEmpty,
		SessionIdleTimeoutSeconds: 600,
	}
}

// ManagedClientOptions starts or reuses the managed runtime.
func ManagedClientOptions(ctx context.Context) (*sdk.ClientOptions, State, error) {
	state, err := Ensure(ctx)
	if err != nil {
		return nil, State{}, err
	}
	return &sdk.ClientOptions{
		Connection: sdk.URIConnection{
			URL:             state.URL,
			ConnectionToken: state.ConnectionToken,
		},
		LogLevel: "error",
		Mode:     sdk.ModeEmpty,
	}, state, nil
}

// Ensure returns a healthy managed runtime, starting one when necessary.
func Ensure(ctx context.Context) (State, error) {
	if state, _, err := Status(ctx); err == nil {
		return state, nil
	}

	dir, err := ensureRuntimeDir()
	if err != nil {
		return State{}, err
	}

	lockPath := filepath.Join(dir, lockFileName)
	lock, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if !errors.Is(err, os.ErrExist) {
			return State{}, fmt.Errorf("acquiring runtime startup lock: %w", err)
		}
		if info, statErr := os.Stat(lockPath); statErr == nil && time.Since(info.ModTime()) > startTimeout {
			if removeErr := os.Remove(lockPath); removeErr == nil {
				return Ensure(ctx)
			}
		}
		return waitForRuntime(ctx, startTimeout)
	}
	lock.Close()
	defer os.Remove(lockPath)

	if state, _, err := Status(ctx); err == nil {
		return state, nil
	}
	_ = os.Remove(filepath.Join(dir, stateFileName))

	executable, err := os.Executable()
	if err != nil {
		return State{}, fmt.Errorf("resolving my-voice executable: %w", err)
	}

	logFile, err := os.OpenFile(filepath.Join(dir, runtimeLogName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return State{}, fmt.Errorf("opening runtime log: %w", err)
	}
	defer logFile.Close()

	cmd := exec.Command(executable, "runtime", "serve")
	configureDetachedProcess(cmd)
	cmd.Stdin = nil
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		return State{}, fmt.Errorf("starting managed runtime supervisor: %w", err)
	}
	if err := cmd.Process.Release(); err != nil {
		return State{}, fmt.Errorf("releasing managed runtime supervisor: %w", err)
	}

	return waitForRuntime(ctx, startTimeout)
}

// Serve owns the persistent Copilot runtime until the context is canceled.
func Serve(ctx context.Context) error {
	dir, err := ensureRuntimeDir()
	if err != nil {
		return err
	}
	token, err := randomToken()
	if err != nil {
		return err
	}

	client := sdk.NewClient(&sdk.ClientOptions{
		BaseDirectory: copilotHome(),
		Connection: sdk.TCPConnection{
			Port:            0,
			ConnectionToken: token,
		},
		LogLevel:                  "error",
		Mode:                      sdk.ModeEmpty,
		SessionIdleTimeoutSeconds: 600,
	})
	if err := client.Start(ctx); err != nil {
		return fmt.Errorf("starting Copilot runtime: %w", err)
	}

	status, err := client.GetStatus(ctx)
	if err != nil {
		_ = StopClient(client)
		return fmt.Errorf("checking Copilot runtime: %w", err)
	}

	state := State{
		SupervisorPID:   os.Getpid(),
		URL:             "127.0.0.1:" + strconv.Itoa(client.RuntimePort()),
		ConnectionToken: token,
		Version:         status.Version,
		ProtocolVersion: status.ProtocolVersion,
		StartedAt:       time.Now().UTC(),
	}
	if err := writeState(filepath.Join(dir, stateFileName), state); err != nil {
		_ = StopClient(client)
		return err
	}

	store, storeErr := NewSessionStore()
	if storeErr == nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = store.CleanupStale(cleanupCtx, client)
		cancel()
	}

	<-ctx.Done()

	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if storeErr == nil {
		_ = store.CleanupStale(cleanupCtx, client)
	}
	cancel()

	removeStateIfOwned(filepath.Join(dir, stateFileName), os.Getpid())
	return StopClient(client)
}

// Status checks both the state file and the runtime RPC endpoint.
func Status(ctx context.Context) (State, *sdk.GetStatusResponse, error) {
	state, err := readState()
	if err != nil {
		return State{}, nil, err
	}
	if err := processAlive(state.SupervisorPID); err != nil {
		return State{}, nil, fmt.Errorf("managed runtime supervisor is not running: %w", err)
	}
	client := sdk.NewClient(&sdk.ClientOptions{
		Connection: sdk.URIConnection{
			URL:             state.URL,
			ConnectionToken: state.ConnectionToken,
		},
		LogLevel: "error",
		Mode:     sdk.ModeEmpty,
	})
	if err := client.Start(ctx); err != nil {
		return State{}, nil, fmt.Errorf("connecting to managed runtime: %w", err)
	}
	status, statusErr := client.GetStatus(ctx)
	stopErr := StopClient(client)
	if statusErr != nil {
		return State{}, nil, fmt.Errorf("checking managed runtime: %w", statusErr)
	}
	if stopErr != nil {
		return State{}, nil, fmt.Errorf("closing managed runtime connection: %w", stopErr)
	}
	return state, status, nil
}

// StopClient suppresses a v1.0.11 SDK close diagnostic that is printed to stdout.
func StopClient(client *sdk.Client) error {
	stdoutStopMu.Lock()
	defer stdoutStopMu.Unlock()

	discard, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return client.Stop()
	}
	original := os.Stdout
	os.Stdout = discard
	stopErr := client.Stop()
	os.Stdout = original
	closeErr := discard.Close()
	return errors.Join(stopErr, closeErr)
}

// Stop signals the managed supervisor and waits for the runtime to exit.
func Stop(ctx context.Context) error {
	state, err := readState()
	if err != nil {
		return err
	}
	if _, _, err := Status(ctx); err != nil {
		_ = removeState()
		return fmt.Errorf("managed runtime is not healthy: %w", err)
	}

	process, err := os.FindProcess(state.SupervisorPID)
	if err != nil {
		return fmt.Errorf("finding runtime supervisor: %w", err)
	}
	if err := signalStop(process); err != nil {
		return fmt.Errorf("stopping runtime supervisor: %w", err)
	}

	deadline := time.NewTimer(stopTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for managed runtime to stop")
		case <-ticker.C:
			if _, _, err := Status(ctx); err != nil {
				_ = removeState()
				return nil
			}
		}
	}
}

func waitForRuntime(ctx context.Context, timeout time.Duration) (State, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var lastErr error
	for {
		select {
		case <-ctx.Done():
			return State{}, ctx.Err()
		case <-timer.C:
			if lastErr == nil {
				lastErr = fmt.Errorf("runtime did not publish its state")
			}
			return State{}, fmt.Errorf("waiting for managed runtime: %w", lastErr)
		case <-ticker.C:
			state, _, err := Status(ctx)
			if err == nil {
				return state, nil
			}
			lastErr = err
		}
	}
}

func ensureRuntimeDir() (string, error) {
	dir, err := RuntimeDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("creating runtime directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", fmt.Errorf("securing runtime directory: %w", err)
	}
	return dir, nil
}

func readState() (State, error) {
	dir, err := RuntimeDir()
	if err != nil {
		return State{}, err
	}
	data, err := os.ReadFile(filepath.Join(dir, stateFileName))
	if err != nil {
		return State{}, fmt.Errorf("reading managed runtime state: %w", err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("decoding managed runtime state: %w", err)
	}
	if state.SupervisorPID <= 0 || state.URL == "" || state.ConnectionToken == "" {
		return State{}, fmt.Errorf("managed runtime state is incomplete")
	}
	return state, nil
}

func writeState(path string, state State) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encoding managed runtime state: %w", err)
	}
	temp, err := os.OpenFile(path+".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("creating managed runtime state: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("writing managed runtime state: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("closing managed runtime state: %w", err)
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		return fmt.Errorf("publishing managed runtime state: %w", err)
	}
	return os.Chmod(path, 0o600)
}

func removeState() error {
	dir, err := RuntimeDir()
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(dir, stateFileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func removeStateIfOwned(path string, pid int) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var state State
	if json.Unmarshal(data, &state) == nil && state.SupervisorPID == pid {
		_ = os.Remove(path)
	}
}

func randomToken() (string, error) {
	token := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, token); err != nil {
		return "", fmt.Errorf("generating runtime connection token: %w", err)
	}
	return hex.EncodeToString(token), nil
}

func copilotHome() string {
	if home := os.Getenv("COPILOT_HOME"); home != "" {
		return home
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".copilot"
	}
	return filepath.Join(home, ".copilot")
}
