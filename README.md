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
- `config.toml` — Persistent defaults for model, reasoning effort, tone, and format
- `profiles/formal-mail.md` — Professional email style
- `profiles/formal-chat.md` — Professional chat message style
- `profiles/casual-mail.md` — Friendly, conversational email style
- `profiles/casual-chat.md` — Informal chat message style

### config.toml

```toml
# Default LLM model
model = "gpt-5.6-luna"

# Reasoning effort: "none", "minimal", "low", "medium", "high", "xhigh", or "max"
reasoning_effort = "low"

# Default tone: "formal" or "casual"
tone = "formal"

# Default format: "mail" or "chat"
format = "mail"
```

CLI flags always override config.toml values. If no config file exists, hardcoded defaults are used (`formal`, `mail`, `gpt-5.6-luna`, and `low` reasoning effort).

## Managed runtime

Normal commands automatically start and reuse a local, token-protected Copilot runtime. Each rewrite still uses a new isolated session, disables tools and ambient context, and permanently deletes its session before successful completion.

```bash
my-voice runtime start
my-voice runtime status
my-voice runtime stop
```

Use `--direct` to bypass the managed runtime and start a one-shot Copilot process. If automatic managed-runtime startup fails, `my-voice` reports a warning and falls back to direct mode.

Responses stream as they arrive when stdout is a terminal. Redirected or piped output remains buffered and is printed atomically after session deletion.

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
| `--reasoning-effort` | Reasoning effort to use (overrides config.toml) |
| `--direct` | Bypass the managed runtime |
| `--timings` | Print lifecycle and token timing diagnostics to stderr |
| `--list` | List available profiles |

## Prerequisites

- [GitHub Copilot CLI](https://docs.github.com/en/copilot) must be installed and authenticated
