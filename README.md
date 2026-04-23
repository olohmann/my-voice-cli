# my-voice

A CLI tool that rewrites your input in a configured voice using GitHub Copilot.

## Installation

```bash
go install github.com/olohmann/my-voice-cli@latest
```

Or build from source:

```bash
go build -o my-voice .
```

## Usage

```bash
# Formal email (default)
echo "tell John the deploy is done" | my-voice --mail --formal

# Casual chat message
echo "ask about project status" | my-voice --chat --casual

# Formal chat
echo "remind team about standup" | my-voice --chat --formal

# Casual email
echo "thank Sarah for the code review" | my-voice --mail --casual
```

## Configuration

Profiles are stored in `~/.config/my-voice/profiles/` as markdown files. Each file is a system prompt that instructs the LLM how to rewrite your input.

### Initialize default profiles

```bash
my-voice --init
```

This creates 4 default profiles:
- `formal-mail.md` — Professional email style
- `formal-chat.md` — Professional chat message style
- `casual-mail.md` — Friendly, conversational email style
- `casual-chat.md` — Informal chat message style

### Customize profiles

Edit the markdown files in `~/.config/my-voice/profiles/` to adjust the voice to your liking.

### List available profiles

```bash
my-voice --list
```

## Flags

| Flag | Description |
|------|-------------|
| `--formal` | Use formal tone (default) |
| `--casual` | Use casual tone |
| `--mail` | Output as email (default) |
| `--chat` | Output as chat message |
| `--profile-dir` | Override config directory |
| `--init` | Initialize default profile files |
| `--model` | LLM model to use (default: gpt-4.1) |
| `--list` | List available profiles |

## Prerequisites

- [GitHub Copilot CLI](https://docs.github.com/en/copilot) must be installed and authenticated
