<!-- Generated: 2026-01-30 | Updated: 2026-01-30 -->

# rrouter

## Purpose

A lightweight routing proxy for Claude Code CLI that enables instant OAuth routing mode switching without restart. Provides automatic failure detection/recovery between Antigravity (thinking models) and Claude OAuth modes.

## Key Files

| File | Description |
|------|-------------|
| `go.mod` | Go module definition (requires Go 1.22+) |
| `config.json` | Default configuration for model mappings (project root) |
| `install.sh` | Installation script with dry-run support |
| `uninstall.sh` | Uninstallation script with purge option |
| `.rrouter-config.example.json` | Example configuration for model mappings |
| `README.md` | Quick start guide and usage documentation |
| `RROUTER_COMPLETE_GUIDE.md` | Comprehensive technical documentation |

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| `cmd/` | Go source code (see `cmd/AGENTS.md`) |
| `services/` | System service files for launchd/systemd (see `services/AGENTS.md`) |

## For AI Agents

### Working In This Directory

- This is a Go 1.22+ project using standard library + fsnotify
- Single binary architecture: `rrouter` with subcommands (`serve`, `start`, `stop`, etc.)
- Build with: `go build -o rrouter ./cmd/rrouter`
- Configuration lives in `~/.rrouter/` (mode file, config.json, logs)

### Testing Requirements

- Run `go test ./...` before committing
- Test install/uninstall scripts with `--dry-run` flag
- Verify proxy health: `curl http://localhost:8316/health`

### Common Patterns

- Mode switching via filesystem: write to `~/.rrouter/mode`
- fsnotify watches `~/.rrouter/` directory for instant config reload
- Auto mode: Antigravity-first with Claude fallback on failure

## Dependencies

### External

- `github.com/fsnotify/fsnotify` - Filesystem event watching
- `golang.org/x/sys` - System calls (indirect)

### Runtime

- Claude Code CLI (consumer of the proxy)
- cliproxyapi on port 8317 (upstream)

<!-- MANUAL: -->
