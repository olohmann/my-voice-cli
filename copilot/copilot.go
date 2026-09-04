package copilot

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	sdk "github.com/github/copilot-sdk/go"
	"github.com/google/uuid"
	"github.com/olohmann/my-voice-cli/runtimehost"
)

// GenerateOptions configures one isolated rewrite request.
type GenerateOptions struct {
	ClientOptions   *sdk.ClientOptions
	Model           string
	ReasoningEffort string
	Stream          bool
	OnDelta         func(string)
	SessionStore    *runtimehost.SessionStore
}

// Timings contains the measured phases of a rewrite.
type Timings struct {
	Connect          time.Duration
	OrphanCleanup    time.Duration
	SessionCreate    time.Duration
	Model            time.Duration
	Delete           time.Duration
	ClientStop       time.Duration
	Total            time.Duration
	TimeToFirstToken time.Duration
	InterTokenMS     *float64
	InputTokens      *int64
	OutputTokens     *int64
	ReasoningTokens  *int64
	ModelID          string
	EffectiveEffort  string
}

// Result is the completed rewrite and its lifecycle measurements.
type Result struct {
	Content  string
	Streamed bool
	Timings  Timings
}

// Generate sends one isolated rewrite request and permanently deletes its session.
func Generate(ctx context.Context, systemPrompt, userInput string, options GenerateOptions) (Result, error) {
	started := time.Now()
	var result Result
	client := sdk.NewClient(options.ClientOptions)

	phase := time.Now()
	if err := client.Start(ctx); err != nil {
		return result, fmt.Errorf("starting copilot client: %w", err)
	}
	result.Timings.Connect = time.Since(phase)

	stopClient := func() error {
		phase := time.Now()
		err := runtimehost.StopClient(client)
		result.Timings.ClientStop = time.Since(phase)
		result.Timings.Total = time.Since(started)
		return err
	}

	if options.SessionStore == nil {
		stopErr := stopClient()
		return result, errors.Join(fmt.Errorf("session cleanup store is required"), stopErr)
	}

	phase = time.Now()
	if err := options.SessionStore.CleanupStale(ctx, client); err != nil {
		stopErr := stopClient()
		return result, errors.Join(fmt.Errorf("cleaning orphaned sessions: %w", err), stopErr)
	}
	result.Timings.OrphanCleanup = time.Since(phase)

	sessionID := runtimehost.SessionID(uuid.NewString())
	if err := options.SessionStore.Mark(sessionID); err != nil {
		stopErr := stopClient()
		return result, errors.Join(err, stopErr)
	}

	phase = time.Now()
	session, err := client.CreateSession(ctx, SessionConfig(sessionID, systemPrompt, options))
	result.Timings.SessionCreate = time.Since(phase)
	if err != nil {
		cleanupErr := deleteSession(client, options.SessionStore, sessionID)
		stopErr := stopClient()
		return result, errors.Join(fmt.Errorf("creating session: %w", err), cleanupErr, stopErr)
	}

	var eventMu sync.Mutex
	var firstDelta time.Time
	sendStarted := time.Time{}
	unsubscribe := session.On(func(event sdk.SessionEvent) {
		eventMu.Lock()
		defer eventMu.Unlock()

		switch data := event.Data.(type) {
		case *sdk.AssistantMessageDeltaData:
			if data.DeltaContent == "" {
				return
			}
			if firstDelta.IsZero() {
				firstDelta = time.Now()
			}
			result.Streamed = true
			if options.OnDelta != nil {
				options.OnDelta(data.DeltaContent)
			}
		case *sdk.AssistantUsageData:
			result.Timings.ModelID = data.Model
			if data.TimeToFirstTokenMs != nil {
				result.Timings.TimeToFirstToken = time.Duration(*data.TimeToFirstTokenMs * float64(time.Millisecond))
			}
			if data.ReasoningEffort != nil {
				result.Timings.EffectiveEffort = *data.ReasoningEffort
			}
			result.Timings.InterTokenMS = data.InterTokenLatencyMs
			result.Timings.InputTokens = data.InputTokens
			result.Timings.OutputTokens = data.OutputTokens
			result.Timings.ReasoningTokens = data.ReasoningTokens
		}
	})

	sendStarted = time.Now()
	response, sendErr := session.SendAndWait(ctx, sdk.MessageOptions{Prompt: userInput})
	result.Timings.Model = time.Since(sendStarted)
	unsubscribe()

	eventMu.Lock()
	if result.Timings.TimeToFirstToken == 0 && !firstDelta.IsZero() {
		result.Timings.TimeToFirstToken = firstDelta.Sub(sendStarted)
	}
	eventMu.Unlock()

	if sendErr == nil {
		if response == nil {
			sendErr = fmt.Errorf("no response received")
		} else if data, ok := response.Data.(*sdk.AssistantMessageData); ok {
			result.Content = data.Content
		} else {
			sendErr = fmt.Errorf("unexpected response type: %T", response.Data)
		}
	}

	phase = time.Now()
	cleanupErr := deleteSession(client, options.SessionStore, sessionID)
	result.Timings.Delete = time.Since(phase)
	stopErr := stopClient()

	if sendErr != nil {
		sendErr = fmt.Errorf("sending message: %w", sendErr)
	}
	return result, errors.Join(sendErr, cleanupErr, stopErr)
}

// SessionConfig builds the minimal no-tools session used for rewriting.
func SessionConfig(sessionID, systemPrompt string, options GenerateOptions) *sdk.SessionConfig {
	return &sdk.SessionConfig{
		SessionID:        sessionID,
		Model:            options.Model,
		ReasoningEffort:  options.ReasoningEffort,
		ReasoningSummary: sdk.ReasoningSummaryNone,
		SystemMessage: &sdk.SystemMessageConfig{
			Mode:    "replace",
			Content: systemPrompt,
		},
		AvailableTools:                     []string{},
		EnableConfigDiscovery:              sdk.Bool(false),
		SkipCustomInstructions:             sdk.Bool(true),
		EnableOnDemandInstructionDiscovery: sdk.Bool(false),
		EnableFileHooks:                    sdk.Bool(false),
		EnableHostGitOperations:            sdk.Bool(false),
		EnableSessionStore:                 sdk.Bool(false),
		EnableSkills:                       sdk.Bool(false),
		EnableSessionTelemetry:             sdk.Bool(false),
		SkipEmbeddingRetrieval:             sdk.Bool(true),
		EmbeddingCacheStorage:              sdk.String("in-memory"),
		Memory: &sdk.MemoryConfiguration{
			Enabled: false,
		},
		InfiniteSessions: &sdk.InfiniteSessionConfig{
			Enabled: sdk.Bool(false),
		},
		Streaming: sdk.Bool(options.Stream),
	}
}

func deleteSession(client runtimehost.SessionCleaner, store *runtimehost.SessionStore, sessionID string) error {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.DeleteSession(cleanupCtx, sessionID); err != nil {
		sessions, listErr := client.ListSessions(cleanupCtx, nil)
		if listErr == nil {
			found := false
			for _, session := range sessions {
				if session.SessionID == sessionID {
					found = true
					break
				}
			}
			if !found {
				return store.Complete(sessionID)
			}
		}
		return fmt.Errorf("permanently deleting session %s: %w", sessionID, errors.Join(err, listErr))
	}
	return store.Complete(sessionID)
}
