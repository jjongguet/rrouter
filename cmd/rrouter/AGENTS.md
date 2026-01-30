<!-- Parent: ../AGENTS.md -->

# cmd/rrouter

## Purpose

Main Go source code for the rrouter binary, implementing both CLI management commands and the HTTP reverse proxy server. Provides zero-downtime mode switching, automatic failure detection, and model name rewriting for Claude Code CLI routing.

## Key Files

| File | Purpose |
|------|---------|
| `cli.go` | Main entry point with command routing and version info |
| `serve.go` | HTTP proxy server with request handling and health endpoint |
| `daemon.go` | Daemon lifecycle management (start/stop/restart/status) |
| `mode.go` | Mode switching commands (antigravity/claude/auto) |
| `auto.go` | Auto mode state machine for failure detection and recovery |
| `watcher.go` | fsnotify-based configuration file watcher |
| `proxy_config.go` | Configuration loading, model mapping, and glob matching |
| `config_cmd.go` | Configuration management subcommands |
| `health.go` | Health check command implementation |
| `help.go` | Help text and usage information |
| `config.json` | Embedded default configuration |
| `auto_test.go` | Unit tests for auto mode logic |
| `proxy_config_test.go` | Unit tests for configuration and model mapping |

## Architecture

### Single Binary Design

The `rrouter` binary serves dual purposes:
1. **CLI Tool**: Commands like `start`, `stop`, `antigravity`, `claude`, `config edit`
2. **Proxy Server**: HTTP reverse proxy running on port 8316

### Request Flow

```
[User runs: rrouter serve]
    ↓
cli.go → cmdServe()
    ↓
serve.go → proxyHandler()
    ↓
1. configWatcher.GetMode() → "auto" (cached, no I/O)
2. autoSwitch.resolveRouting("auto") → "antigravity" or "claude"
3. modifyRequestBody() → apply model name rewriting
4. proxy.ServeHTTP() → forward to upstream
5. recordUpstreamResponse() → update auto state
```

### Auto Mode State Machine

```
Default: antigravity
    ↓
Failure Detection:
- HTTP 429/5xx × 3 → trigger switch
- Timeout × 2 → trigger switch
- Success → reset counters
    ↓
Switch to: claude
    ↓
Start Cooldown Timer:
- 1st switch: 30min
- 2nd switch: 60min
- 3rd switch: 120min
- Max: 240min
    ↓
Timer Expires → retry antigravity
    ↓
Success → stay on antigravity
Failure → switch back to claude (cooldown ×2)
```

## Component Details

### CLI Entry Point (`cli.go`)

**Version**: 4.0.0

**Commands**:
- `serve`: Run proxy server in foreground
- `start/stop/restart`: Daemon management
- `status`: Show mode and service status
- `antigravity/ag`: Switch to Antigravity mode
- `claude/c`: Switch to Claude mode
- `auto/a`: Activate auto mode
- `config [edit|reset|path]`: Configuration management
- `health/check/--check`: Health check
- `help/--help/-h`: Show help

### Proxy Server (`serve.go`)

**Key Functions**:
- `cmdServe()`: Main server entry point
- `proxyHandler()`: HTTP request handler
- `serveHealthHandler()`: `/health` endpoint
- `modifyRequestBody()`: Model name rewriting
- `createReverseProxy()`: Reverse proxy setup

**Features**:
- Request counting with atomic operations
- Mode-aware routing (auto → concrete target)
- Model name rewriting via glob patterns
- Health endpoint with auto-switch state
- Graceful shutdown on SIGTERM/SIGINT
- Date-based logging to `~/.rrouter/logs/YYYY-MM-DD.log`

### Auto Mode Logic (`auto.go`)

**Constants**:
```go
failureThreshold = 3  // consecutive 429/5xx
timeoutThreshold = 2  // consecutive timeouts
initialCooldown  = 30 * time.Minute
maxCooldown      = 4 * time.Hour
```

**State Struct**:
```go
type autoState struct {
    failureCount     int
    timeoutCount     int
    switched         bool
    switchedAt       time.Time
    cooldownDuration time.Duration
    cooldownTimer    *time.Timer
    generation       uint64  // prevent stale timer race
    switchCount      atomic.Int64
    healthySince     time.Time
}
```

**Key Methods**:
- `resolveRouting(intent)`: Map "auto" → actual target
- `recordUpstreamResponse(statusCode, isTimeout)`: Update state
- `triggerSwitch(reason)`: Execute failover
- `startCooldown()`: Schedule recovery attempt
- `reset()`: Clear state on manual mode switch
- `HealthInfo()`: Export state for /health endpoint

### Configuration Watcher (`watcher.go`)

**Purpose**: Zero I/O configuration access via fsnotify

**Mechanism**:
- Watches `~/.rrouter/` directory (not individual files)
- Handles macOS inode replacement on file write
- Caches mode and config in memory
- Reloads only on filesystem events

**Events Handled**:
- `mode` file change → reload mode, reset auto state if needed
- `config.json` change → reload configuration

**Fallback**: If fsnotify fails, falls back to per-request file reads

### Configuration Management (`proxy_config.go`)

**Embedded Config**: Uses `//go:embed config.json` for defaults

**Structures**:
```go
type Config struct {
    Modes       map[string]ModeConfig
    DefaultMode string
}

type ModeConfig struct {
    Mappings []ModelMapping
}

type ModelMapping struct {
    Match   string  // glob pattern
    Rewrite string  // target model
}
```

**Model Matching**:
- Uses `filepath.Match()` for glob patterns
- Supports wildcards: `claude-sonnet-*`, `claude-*`, `*`
- First match wins
- No match → passthrough

### Daemon Management (`daemon.go`)

**Features**:
- PID file management at `~/.rrouter/rrouter.pid`
- Legacy PID file migration from `rrouterd.pid`
- Dual detection: PID file + port check (for launchd)
- Clean shutdown with SIGTERM → SIGKILL fallback
- Log file at `~/.rrouter/logs/daemon.log`

**Status Display**:
- Current mode with description
- Auto-switch state (if in auto mode)
- Service status: rrouter and clipproxyapi
- Port information

### Mode Switching (`mode.go`)

**Commands**:
- `cmdAntigravity()`: Write "antigravity" to mode file
- `cmdClaude()`: Write "claude" to mode file
- `cmdAuto()`: Write "auto" to mode file

**Side Effects**:
- Writes to `~/.rrouter/mode`
- fsnotify watcher detects change
- Proxy reloads mode automatically
- Auto state resets if leaving auto mode

## For AI Agents

When working with this code:

1. **Understand the Dual Nature**: Same binary serves as CLI and server. Check `os.Args[1]` in `cli.go` for command routing.

2. **Auto Mode Complexity**:
   - Uses generation counter to prevent timer race conditions
   - Tracks timeout and HTTP failures separately
   - Cooldown escalates exponentially (30m → 240m max)
   - Resets cooldown after 2× sustained health

3. **Configuration Paths**:
   - Embedded default: `config.json` in this directory
   - User config: `~/.rrouter/config.json`
   - Mode file: `~/.rrouter/mode`
   - Logs: `~/.rrouter/logs/`

4. **Testing**:
   - Auto mode tests: `auto_test.go`
   - Config tests: `proxy_config_test.go`
   - Run tests: `go test -v`
   - Manual testing: `go build && ./rrouter serve`

5. **Common Modifications**:
   - Adjust failure thresholds: Edit constants in `auto.go`
   - Add new mode: Update embedded `config.json`, add CLI command
   - Change model mappings: Users edit `~/.rrouter/config.json`
   - Modify health output: Update `serveHealthHandler()` in `serve.go`

6. **Key Patterns**:
   - Mutex for config watcher (`sync.RWMutex`)
   - Atomic operations for counters (`atomic.Uint64`, `atomic.Int64`)
   - Context-based per-request data (proxy error tracking)
   - Timer generation for race-free cooldown
   - Embedded defaults with user override

7. **Debugging**:
   - Run `rrouter serve` in foreground to see logs
   - Check `/health` endpoint for state
   - Look for `[AUTO]`, `[WATCHER]`, `[Mode]` prefixes in logs
   - Verify fsnotify events trigger reloads
