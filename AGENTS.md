# rrouter

## Purpose

Lightweight routing proxy for Claude Code CLI that enables zero-downtime OAuth routing mode switching. Provides intelligent routing between Antigravity (Gemini Thinking models) and Claude OAuth with automatic failure detection and recovery.

## Key Files

| File | Purpose |
|------|---------|
| `README.md` | Quick start guide and basic documentation |
| `README.ko.md` | Korean language documentation |
| `go.mod` | Go module definition and dependencies |
| `go.sum` | Dependency checksums |
| `config.json` | Default configuration with model mapping rules |
| `install.sh` | Automated installation script |
| `uninstall.sh` | Automated uninstall script |
| `LICENSE` | MIT License |
| `CODE_OF_CONDUCT.md` | Community code of conduct |

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| `cmd/rrouter/` | Main Go source code for CLI and proxy server |
| `services/` | Systemd and launchd service configuration files |
| `docs/` | Comprehensive documentation and guides |

## Project Overview

**rrouter** is a Go-based routing proxy that sits between Claude Code CLI and clipproxyapi, enabling:

- **Three routing modes**: antigravity, claude, auto
- **Zero-downtime mode switching** via fsnotify file watching
- **Automatic failure detection** with bidirectional failover
- **Model name rewriting** using glob patterns
- **Health monitoring** endpoint for status tracking

### Architecture

```
Claude Code CLI (ANTHROPIC_BASE_URL=localhost:8316)
    ↓
rrouter:8316 (mode switching, model rewriting, auto fallback)
    ↓
clipproxyapi:8317 (oauth-model-alias matching)
    ↓
Antigravity / Claude OAuth
```

### Core Features

1. **Mode Management**
   - `antigravity`: Route through Gemini Thinking models
   - `claude`: Direct OAuth passthrough
   - `auto`: Intelligent failover with cooldown

2. **Configuration**
   - File-based mode switching (`~/.rrouter/mode`)
   - JSON configuration (`~/.rrouter/config.json`)
   - Glob pattern matching for model names
   - Environment variables for ports and upstream

3. **Auto Mode Failover**
   - Detects consecutive failures (3× 4xx/5xx or 2× timeout)
   - Switches to alternate target automatically
   - Cooldown periods: 30min → 60min → 120min → 240min (max)
   - Bidirectional recovery between antigravity ↔ claude

4. **Daemon Management**
   - Unified `rrouter start/stop/restart/status` commands
   - systemd (Linux) and launchd (macOS) support
   - PID file management with fallback port checking

## For AI Agents

When working with this codebase:

1. **Single Binary Architecture**: The `rrouter` binary contains both CLI tools and the proxy server. Use `rrouter serve` for the server, other commands for management.

2. **Core Components**:
   - `cmd/rrouter/serve.go`: HTTP proxy handler with request/response logging
   - `cmd/rrouter/auto.go`: Auto mode state machine for failure detection
   - `cmd/rrouter/watcher.go`: fsnotify-based configuration file watcher
   - `cmd/rrouter/proxy_config.go`: Configuration loading and model mapping

3. **Key Concepts**:
   - **Mode vs Target**: Mode is user intent (antigravity/claude/auto), target is actual routing destination
   - **Intent Resolution**: Auto mode resolves to concrete target based on failure state
   - **Cooldown Generation**: Uses generation counter to prevent stale timer race conditions

4. **Testing Approach**:
   - Run `rrouter serve` in foreground for development
   - Check `/health` endpoint for state inspection
   - Monitor logs for mode changes and auto-switch events
   - Test mode switching without restart

5. **Common Tasks**:
   - Add new mode: Update `config.json`, add CLI command in `mode.go`
   - Modify failover logic: Edit `auto.go` thresholds and cooldown constants
   - Change model mappings: Edit user's `~/.rrouter/config.json`
   - Debug issues: Check logs with `journalctl --user -u rrouter -f` (Linux) or `~/.rrouter/logs/` (macOS)

6. **Important Patterns**:
   - All configuration changes are detected via fsnotify (no manual reload needed)
   - Auto mode state is in-memory only (proxy restart = fresh start)
   - Glob patterns use Go's `filepath.Match` syntax
   - Health endpoint returns different fields based on active mode
