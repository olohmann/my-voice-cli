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

Settings are stored in `~/.config/my-voice/` (respects `XDG_CONFIG_HOME`).

### Initialize config and profiles

```bash
my-voice --init
```

This creates:
- `config.toml` — Persistent defaults for model, tone, and format
- `profiles/formal-mail.md` — Professional email style
- `profiles/formal-chat.md` — Professional chat message style
- `profiles/casual-mail.md` — Friendly, conversational email style
- `profiles/casual-chat.md` — Informal chat message style

### config.toml

```toml
# Default LLM model
model = "gpt-4.1"

# Default tone: "formal" or "casual"
tone = "formal"

# Default format: "mail" or "chat"
format = "mail"
```

CLI flags always override config.toml values. If no config file exists, hardcoded defaults are used (`formal`, `mail`, `gpt-4.1`).

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
| `--init` | Initialize default config and profile files |
| `--model` | LLM model to use (overrides config.toml) |
| `--list` | List available profiles |

## Prerequisites

- [GitHub Copilot CLI](https://docs.github.com/en/copilot) must be installed and authenticated
