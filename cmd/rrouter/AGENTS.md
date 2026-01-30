<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-01-30 | Updated: 2026-01-30 -->

# rrouter (cmd/rrouter)

## Purpose

Main Go application implementing a single binary with CLI subcommands for routing proxy management. Handles HTTP proxying, mode switching, daemon lifecycle, and automatic failover logic.

## Key Files

| File | Description |
|------|-------------|
| `cli.go` | Entry point, command routing (main function) |
| `serve.go` | HTTP proxy server, request handling, `/health` endpoint |
| `daemon.go` | Daemon lifecycle: start, stop, restart, status commands |
| `mode.go` | Mode switching commands: antigravity, claude, auto |
| `auto.go` | Auto mode state machine: failure detection, cooldown, recovery |
| `watcher.go` | fsnotify-based config watcher for hot reload |
| `proxy_config.go` | Configuration loading, model rewriting logic |
| `config_cmd.go` | Config subcommands: view, edit, reset, path |
| `health.go` | Health check command implementation |
| `help.go` | Help text and version display |
| `auto_test.go` | Tests for auto mode logic |
| `proxy_config_test.go` | Tests for config and model matching |

## Architecture

```
cli.go (main)
    ├── serve.go     → HTTP proxy server (foreground)
    ├── daemon.go    → start/stop/restart/status
    ├── mode.go      → antigravity/claude/auto
    ├── config_cmd.go → config subcommands
    └── health.go    → health check

serve.go uses:
    ├── proxy_config.go  → Config loading, model rewriting
    ├── watcher.go       → fsnotify for hot reload
    └── auto.go          → Auto mode state machine
```

## For AI Agents

### Working In This Directory

- All files are in package `main`
- Global state in `serve.go`: `appConfig`, `configWatcher`, `autoSwitch`
- PID file: `~/.rrouter/rrouter.pid`
- Mode file: `~/.rrouter/mode`
- Logs: `~/.rrouter/logs/YYYY-MM-DD.log`

### Key Modification Points

| To Change | Edit File |
|-----------|-----------|
| Add new CLI command | `cli.go` (switch statement) + new handler |
| Modify proxy behavior | `serve.go` (proxyHandler function) |
| Change auto mode thresholds | `auto.go` (constants at top) |
| Add model mappings | `config.json` (project root) or `~/.rrouter/config.json` |
| Change daemon behavior | `daemon.go` |

### Testing Requirements

```bash
go test ./cmd/rrouter/...
```

Key test files:
- `auto_test.go` - Tests failure detection, cooldown, recovery
- `proxy_config_test.go` - Tests glob matching, model rewriting

### Common Patterns

- **Mode resolution**: `configWatcher.GetMode()` returns cached mode
- **Auto routing**: `autoSwitch.resolveRouting(intent)` maps "auto" to concrete target
- **Model rewriting**: `rewriteModelWithConfig()` uses glob patterns
- **Graceful shutdown**: SIGTERM/SIGINT handlers in `serve.go`

## Dependencies

### Internal

- Uses Go standard library extensively (net/http, encoding/json, os, path/filepath)
- Shares config types between proxy_config.go and watcher.go

### External

- `github.com/fsnotify/fsnotify` - Used in watcher.go for directory watching

<!-- MANUAL: -->
