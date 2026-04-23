package copilot

import (
	"context"
	"fmt"

	sdk "github.com/github/copilot-sdk/go"
)

// Generate sends the user's input to Copilot with the given system prompt and returns the response.
func Generate(ctx context.Context, systemPrompt, userInput, model string) (string, error) {
	client := sdk.NewClient(nil)

	if err := client.Start(ctx); err != nil {
		return "", fmt.Errorf("starting copilot client: %w", err)
	}
	defer client.Stop()

	session, err := client.CreateSession(ctx, &sdk.SessionConfig{
		Model: model,
		SystemMessage: &sdk.SystemMessageConfig{
			Mode:    "replace",
			Content: systemPrompt,
		},
	})
	if err != nil {
		return "", fmt.Errorf("creating session: %w", err)
	}
	defer session.Disconnect()

	result, err := session.SendAndWait(ctx, sdk.MessageOptions{
		Prompt: userInput,
	})
	if err != nil {
		return "", fmt.Errorf("sending message: %w", err)
	}

	if result == nil {
		return "", fmt.Errorf("no response received")
	}

	if d, ok := result.Data.(*sdk.AssistantMessageData); ok {
		return d.Content, nil
	}

	return "", fmt.Errorf("unexpected response type: %T", result.Data)
}
