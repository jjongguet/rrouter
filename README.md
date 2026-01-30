[한국어](README.ko.md) | **English**

# rrouter

A lightweight routing proxy for Claude Code CLI. Switch OAuth routing modes instantly without restart, with automatic failure detection and recovery.

> **Note**: rrouter is designed to work together with [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI). rrouter handles model name rewriting and mode switching (Stage 1), while CLIProxyAPI handles OAuth channel routing (Stage 2).

## Features

- **Zero-downtime mode switching** - Change routing modes instantly without restarting proxy or Claude Code CLI
- **Automatic failure detection** - Bidirectional fallback between Antigravity and Claude on consecutive failures
- **fsnotify-based configuration watching** - Instant file system change reflection with zero I/O overhead
- **Glob pattern matching** - Wildcard support in model name mapping (`claude-sonnet-*`)
- **Three routing modes** - antigravity (Gemini Thinking), claude (OAuth passthrough), auto (intelligent fallback)
- **Health monitoring** - Built-in health endpoint for status tracking

## Installation

```bash
./install.sh
```

This script will:
- Build the binary
- Install to `/usr/local/bin/rrouter`
- Set up daemon service (systemd/launchd)
- Create config directory at `~/.rrouter/`
- Configure default mode as `auto`

## Configuration

### Claude Code Settings (Required)

Edit `~/.claude/settings.json`:

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://localhost:8316"
  }
}
```

If the file doesn't exist:

```bash
mkdir -p ~/.claude
cat > ~/.claude/settings.json <<'EOF'
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://localhost:8316"
  }
}
EOF
```

### rrouter Configuration

**Mode file**: `~/.rrouter/mode` (valid values: `antigravity`, `claude`, `auto`)

**Config file**: `~/.rrouter/config.json` (optional, uses embedded defaults if missing)

Example config:

```json
{
  "modes": {
    "antigravity": {
      "mappings": [
        {
          "match": "claude-sonnet-*",
          "rewrite": "gemini-claude-sonnet-4-5-thinking"
        },
        {
          "match": "claude-opus-*",
          "rewrite": "gemini-claude-opus-4-5-thinking"
        },
        {
          "match": "claude-haiku-*",
          "rewrite": "gemini-3-flash-preview"
        }
      ]
    },
    "claude": {
      "mappings": []
    }
  },
  "defaultMode": "claude"
}
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `RROUTER_PORT` | 8316 | Proxy listening port |
| `RROUTER_UPSTREAM` | http://localhost:8317 | Upstream URL (clipproxyapi) |

## Usage

### Basic Commands

```bash
rrouter start      # Start daemon in background
rrouter stop       # Stop daemon
rrouter restart    # Restart daemon
rrouter status     # Show current mode and daemon status
```

### Mode Switching

```bash
rrouter antigravity    # Switch to Antigravity mode (alias: ag)
rrouter claude         # Switch to Claude OAuth mode (alias: c)
rrouter auto           # Switch to Auto mode with fallback (alias: a)
```

### Health Check

```bash
rrouter check          # CLI health check (aliases: health, --check)
curl http://localhost:8316/health | jq '.'
```

### Configuration Management

```bash
rrouter config         # View current config
rrouter config edit    # Edit config with $EDITOR
rrouter config reset   # Reset to defaults
rrouter config path    # Show config file path
```

## Routing Modes

| Mode | Description | Use Case |
|------|-------------|----------|
| **antigravity** | Use Gemini Thinking models | Reasoning/analysis tasks requiring high quality |
| **claude** | Direct Claude OAuth usage | Fast response, lower cost |
| **auto** (recommended) | Bidirectional automatic fallback | General use with automatic recovery |

### Auto Mode

Auto mode provides intelligent bidirectional fallback:

- **Failure detection**: 3 consecutive 4xx/5xx errors or 2 timeouts triggers switch
- **Cooldown policy**: 30min → 60min → 120min → 240min (max)
- **Bidirectional**: Antigravity ↔ Claude in both directions
- **Success reset**: 2× cooldown period of healthy operation resets to 30min

Health endpoint shows auto state:

```bash
curl -s http://localhost:8316/health | jq '.autoSwitch'
```

## CLI Commands

| Command | Alias | Description |
|---------|-------|-------------|
| `rrouter serve` | - | Run proxy server in foreground |
| `rrouter start` | - | Start proxy daemon in background |
| `rrouter stop` | - | Stop proxy daemon |
| `rrouter restart` | - | Restart proxy daemon |
| `rrouter status` | - | Show current mode and daemon status |
| `rrouter antigravity` | `ag` | Switch to Antigravity mode |
| `rrouter claude` | `c` | Switch to Claude OAuth mode |
| `rrouter auto` | `a` | Activate Auto mode |
| `rrouter check` | `health`, `--check` | Health check |
| `rrouter config` | - | View current config.json |
| `rrouter config edit` | - | Edit config.json with editor |
| `rrouter config reset` | - | Reset config.json to defaults |
| `rrouter config path` | - | Show config.json file path |
| `rrouter help` | `--help`, `-h` | Show help |

## Architecture

```
┌─────────────────────┐
│  Claude Code CLI    │  ANTHROPIC_BASE_URL=http://localhost:8316
└──────────┬──────────┘
           │ POST /v1/messages
           ▼
┌─────────────────────┐
│   rrouter:8316      │  - Mode switching (antigravity/claude/auto)
│                     │  - Model name rewriting (glob patterns)
│                     │  - Auto failure detection & fallback
└──────────┬──────────┘
           │ Modified request
           ▼
┌─────────────────────┐
│ clipproxyapi:8317   │  - oauth-model-alias matching
│                     │  - OAuth channel routing
└──────────┬──────────┘
           │
           ▼
   Antigravity / Claude OAuth
```

## Requirements

- Go 1.22+ (build only)
- clipproxyapi running on localhost:8317
- Linux (systemd) or macOS (launchd)

## Development

### Build

```bash
go build -o rrouter ./cmd/rrouter
```

### Test

```bash
# Run in foreground
./rrouter serve

# In another terminal
curl http://localhost:8316/health
```

## References

- [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) - The upstream proxy that rrouter forwards to
- [oh-my-claudecode](https://github.com/Yeachan-Heo/oh-my-claudecode) - Multi-agent orchestration for Claude Code

## License

MIT

---

For detailed documentation including clipproxyapi integration, two-stage model transformation, and advanced troubleshooting, see the full bilingual documentation in the repository.
