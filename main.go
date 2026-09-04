package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/olohmann/my-voice-cli/config"
	"github.com/olohmann/my-voice-cli/copilot"
	"github.com/olohmann/my-voice-cli/runtimehost"
	"github.com/olohmann/my-voice-cli/spinner"
	flag "github.com/spf13/pflag"
	"golang.org/x/term"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "runtime" {
		os.Exit(runRuntimeCommand(os.Args[2:]))
	}

	// Tone flags
	formal := flag.Bool("formal", false, "Use formal tone")
	casual := flag.Bool("casual", false, "Use casual tone")

	// Format flags
	mail := flag.Bool("mail", false, "Output as email")
	chat := flag.Bool("chat", false, "Output as chat message")

	// Other flags
	profileDir := flag.String("profile-dir", "", "Override config directory")
	initCmd := flag.Bool("init", false, "Initialize default config and profile files")
	model := flag.String("model", "", "LLM model to use (overrides config.toml)")
	reasoningEffort := flag.String("reasoning-effort", "", "Reasoning effort to use (overrides config.toml)")
	direct := flag.Bool("direct", false, "Start a one-shot Copilot runtime instead of using the managed runtime")
	timings := flag.Bool("timings", false, "Print lifecycle timing diagnostics to stderr")
	list := flag.Bool("list", false, "List available profiles")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  my-voice [flags] < input\n")
		fmt.Fprintf(os.Stderr, "  my-voice runtime <start|stop|status>\n\n")
		fmt.Fprintf(os.Stderr, "Rewrites stdin input in a configured voice using GitHub Copilot.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  echo \"tell John the deploy is done\" | my-voice --mail --formal\n")
		fmt.Fprintf(os.Stderr, "  echo \"ask about project status\" | my-voice --chat --casual\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	configDir := config.ConfigDir(*profileDir)

	// Load persistent config (config.toml)
	cfg, err := config.LoadConfig(configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v (using defaults)\n", err)
		cfg = config.DefaultConfig()
	}

	// Handle --init
	if *initCmd {
		fmt.Fprintf(os.Stderr, "Initializing config in %s\n", configDir)
		if err := config.Init(configDir); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Done!\n")
		return
	}

	// Handle --list
	if *list {
		profiles, err := config.ListProfiles(configDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Available profiles:")
		for _, p := range profiles {
			fmt.Printf("  %s\n", p)
		}
		return
	}

	// Resolve tone: CLI flags override config.toml
	if *formal && *casual {
		fmt.Fprintf(os.Stderr, "Error: --formal and --casual are mutually exclusive\n")
		os.Exit(1)
	}
	tone := cfg.Tone
	if *formal {
		tone = "formal"
	} else if *casual {
		tone = "casual"
	}

	// Resolve format: CLI flags override config.toml
	if *mail && *chat {
		fmt.Fprintf(os.Stderr, "Error: --mail and --chat are mutually exclusive\n")
		os.Exit(1)
	}
	format := cfg.Format
	if *mail {
		format = "mail"
	} else if *chat {
		format = "chat"
	}

	// Resolve model: CLI flag overrides config.toml
	activeModel := cfg.Model
	if *model != "" {
		activeModel = *model
	}
	activeReasoningEffort := cfg.ReasoningEffort
	if *reasoningEffort != "" {
		activeReasoningEffort = *reasoningEffort
	}
	activeReasoningEffort, err = config.ValidateReasoningEffort(activeReasoningEffort)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Set up context with signal handling early so Ctrl-C works during input
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Read stdin
	var userInput string
	stat, _ := os.Stdin.Stat()
	if stat.Mode()&os.ModeCharDevice != 0 {
		// Interactive: raw terminal mode for reliable Ctrl-D handling
		fd := int(os.Stdin.Fd())
		oldState, err := term.MakeRaw(fd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot set raw terminal mode: %v\n", err)
			os.Exit(1)
		}
		defer term.Restore(fd, oldState)

		fmt.Fprint(os.Stderr, "Enter your text (Ctrl-D to send, Ctrl-C to cancel):\r\n")

		var buf []byte
		b := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(b)
			if n == 0 || err != nil {
				break
			}
			switch b[0] {
			case 4: // Ctrl-D — submit
				fmt.Fprint(os.Stderr, "\r\n")
				goto done
			case 3: // Ctrl-C — cancel
				term.Restore(fd, oldState)
				fmt.Fprint(os.Stderr, "\r\n")
				os.Exit(130)
			case 13: // Enter (CR in raw mode)
				buf = append(buf, '\n')
				fmt.Fprint(os.Stderr, "\r\n")
			case 127, 8: // Backspace / Delete
				if len(buf) > 0 {
					buf = buf[:len(buf)-1]
					fmt.Fprint(os.Stderr, "\b \b")
				}
			default:
				if b[0] >= 32 { // printable chars
					buf = append(buf, b[0])
					os.Stderr.Write(b[:1])
				}
			}
		}
	done:
		term.Restore(fd, oldState)
		userInput = string(buf)
		fmt.Fprintln(os.Stderr, "Processing...")
	} else {
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
			os.Exit(1)
		}
		userInput = string(input)
	}
	if len(strings.TrimSpace(userInput)) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no input provided. Pipe text to stdin.\n")
		flag.Usage()
		os.Exit(1)
	}

	// Load profile
	systemPrompt, err := config.LoadProfile(configDir, tone, format)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading profile: %v\n", err)
		os.Exit(1)
	}

	sessionStore, err := runtimehost.NewSessionStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error preparing session cleanup: %v\n", err)
		os.Exit(1)
	}

	runtimeStarted := time.Now()
	clientOptions := runtimehost.DirectClientOptions()
	if !*direct {
		stopRuntimeSpinner := spinner.Start("Starting Copilot...")
		managedOptions, _, runtimeErr := runtimehost.ManagedClientOptions(ctx)
		stopRuntimeSpinner()
		if runtimeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: managed runtime unavailable: %v (using direct mode)\n", runtimeErr)
		} else {
			clientOptions = managedOptions
		}
	}
	runtimeSetup := time.Since(runtimeStarted)

	// Generate response
	stop := spinner.Start("Thinking...")
	stream := term.IsTerminal(int(os.Stdout.Fd()))
	result, err := copilot.Generate(ctx, systemPrompt, userInput, copilot.GenerateOptions{
		ClientOptions:   clientOptions,
		Model:           activeModel,
		ReasoningEffort: activeReasoningEffort,
		Stream:          stream,
		OnDelta: func(delta string) {
			stop()
			fmt.Print(delta)
		},
		SessionStore: sessionStore,
	})
	stop()
	if err != nil {
		if result.Streamed {
			fmt.Fprintln(os.Stderr)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if !result.Streamed {
		fmt.Print(result.Content)
	}
	if *timings {
		printTimings(runtimeSetup, result.Timings)
	}
}

func runRuntimeCommand(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "Usage: my-voice runtime <start|stop|status>")
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	switch args[0] {
	case "start":
		state, err := runtimehost.Ensure(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error starting managed runtime: %v\n", err)
			return 1
		}
		fmt.Printf("Managed runtime running at %s (Copilot %s, PID %d)\n", state.URL, state.Version, state.SupervisorPID)
		return 0
	case "stop":
		if err := runtimehost.Stop(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Error stopping managed runtime: %v\n", err)
			return 1
		}
		fmt.Println("Managed runtime stopped")
		return 0
	case "status":
		state, status, err := runtimehost.Status(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Managed runtime is not running: %v\n", err)
			return 1
		}
		fmt.Printf("Managed runtime running at %s (Copilot %s, protocol %d, PID %d)\n",
			state.URL, status.Version, status.ProtocolVersion, state.SupervisorPID)
		return 0
	case "serve":
		if err := runtimehost.Serve(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Managed runtime failed: %v\n", err)
			return 1
		}
		return 0
	default:
		fmt.Fprintln(os.Stderr, "Usage: my-voice runtime <start|stop|status>")
		return 2
	}
}

func printTimings(runtimeSetup time.Duration, timings copilot.Timings) {
	fmt.Fprintf(os.Stderr,
		"\nTimings: runtime=%s connect=%s orphan-cleanup=%s session=%s model=%s delete=%s client-stop=%s total=%s",
		runtimeSetup.Round(time.Millisecond),
		timings.Connect.Round(time.Millisecond),
		timings.OrphanCleanup.Round(time.Millisecond),
		timings.SessionCreate.Round(time.Millisecond),
		timings.Model.Round(time.Millisecond),
		timings.Delete.Round(time.Millisecond),
		timings.ClientStop.Round(time.Millisecond),
		(runtimeSetup + timings.Total).Round(time.Millisecond),
	)
	if timings.TimeToFirstToken > 0 {
		fmt.Fprintf(os.Stderr, " ttft=%s", timings.TimeToFirstToken.Round(time.Millisecond))
	}
	if timings.InterTokenMS != nil {
		fmt.Fprintf(os.Stderr, " inter-token=%.1fms", *timings.InterTokenMS)
	}
	if timings.InputTokens != nil {
		fmt.Fprintf(os.Stderr, " input-tokens=%d", *timings.InputTokens)
	}
	if timings.OutputTokens != nil {
		fmt.Fprintf(os.Stderr, " output-tokens=%d", *timings.OutputTokens)
	}
	if timings.ReasoningTokens != nil {
		fmt.Fprintf(os.Stderr, " reasoning-tokens=%d", *timings.ReasoningTokens)
	}
	if timings.ModelID != "" {
		fmt.Fprintf(os.Stderr, " model=%s", timings.ModelID)
	}
	if timings.EffectiveEffort != "" {
		fmt.Fprintf(os.Stderr, " effort=%s", timings.EffectiveEffort)
	}
	fmt.Fprintln(os.Stderr)
}
