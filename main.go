package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/olohmann/my-voice-cli/config"
	"github.com/olohmann/my-voice-cli/copilot"
)

func main() {
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
	list := flag.Bool("list", false, "List available profiles")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: my-voice [flags] < input\n\n")
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

	// Read stdin
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
		os.Exit(1)
	}
	userInput := string(input)
	if len(userInput) == 0 {
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

	// Set up context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	// Generate response
	response, err := copilot.Generate(ctx, systemPrompt, userInput, activeModel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(response)
}
