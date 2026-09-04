---
name: using-my-voice-cli
description: "Rewrites text in a configured voice using the my-voice CLI. Covers installation, CLI flags (--formal/--casual, --mail/--chat, --model, --init, --list), configuration via config.toml and custom profiles, and piping input via stdin. Use when asking how to rewrite text as emails or chat messages in different tones, configure my-voice profiles, or troubleshoot my-voice CLI usage."
---

# Using my-voice CLI

`my-voice` rewrites piped-in text in a chosen voice. Two axes control the output:

- **Tone**: `--formal` or `--casual`
- **Format**: `--mail` (email) or `--chat` (chat message)

## Installation

```bash
go install github.com/olohmann/my-voice-cli@latest
```

For local development, clone the repo and use `make install` which symlinks the built binary into `~/.local/bin` (override with `PREFIX`). After a local code change, run `make install` again to rebuild and the symlink picks up the new binary automatically.

## Quick start

```bash
# Formal email (default)
echo "tell John the deploy is done" | my-voice

# Casual chat message
echo "ask about project status" | my-voice --chat --casual

# Formal chat
echo "remind team about standup" | my-voice --chat --formal

# Casual email
echo "thank Sarah for the code review" | my-voice --mail --casual
```

### Interactive mode

Run `my-voice` without piping to type input directly. Press **Ctrl-D** to submit, or **Ctrl-C** to cancel.

```bash
my-voice --chat --casual
# Type your message, then press Ctrl-D
```

## Flags

| Flag | Description |
|------|-------------|
| `--formal` | Use formal tone (default) |
| `--casual` | Use casual tone |
| `--mail` | Output as email (default) |
| `--chat` | Output as chat message |
| `--model` | LLM model to use (overrides config.toml) |
| `--profile-dir` | Override config directory |
| `--init` | Initialize default config and profile files |
| `--list` | List available profiles |

`--formal` and `--casual` are mutually exclusive. So are `--mail` and `--chat`.

## Configuration

### Initialize

```bash
my-voice --init
```

Creates `config.toml` and four default profile files in `~/.config/my-voice/` (respects `XDG_CONFIG_HOME`).

### config.toml

```toml
# Default LLM model
model = "gpt-5.6-luna"

# Default tone: "formal" or "casual"
tone = "formal"

# Default format: "mail" or "chat"
format = "mail"
```

CLI flags always override config.toml values.

### Custom profiles

Edit the markdown files in the profiles directory to adjust the voice:

- `formal-mail.md` — professional email style
- `formal-chat.md` — professional chat message style
- `casual-mail.md` — friendly email style
- `casual-chat.md` — informal chat message style

Each profile is a system prompt that instructs the model how to rewrite the input.

### List profiles

```bash
my-voice --list
```

Shows all available profiles, marking custom ones with `(custom)` and built-in ones with `(default)`.
