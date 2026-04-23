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
	initProfiles := flag.Bool("init", false, "Initialize default profile files in config dir")
	model := flag.String("model", "gpt-4.1", "LLM model to use")
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

	// Handle --init
	if *initProfiles {
		fmt.Fprintf(os.Stderr, "Initializing profiles in %s\n", config.ProfilesDir(configDir))
		if err := config.InitProfiles(configDir); err != nil {
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

	// Resolve tone
	tone := "formal"
	if *formal && *casual {
		fmt.Fprintf(os.Stderr, "Error: --formal and --casual are mutually exclusive\n")
		os.Exit(1)
	}
	if *casual {
		tone = "casual"
	}

	// Resolve format
	format := "mail"
	if *mail && *chat {
		fmt.Fprintf(os.Stderr, "Error: --mail and --chat are mutually exclusive\n")
		os.Exit(1)
	}
	if *chat {
		format = "chat"
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
	response, err := copilot.Generate(ctx, systemPrompt, userInput, *model)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(response)
}
