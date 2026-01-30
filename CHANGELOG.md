# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [4.1.0] - 2026-01-30

### Added

- Agent-based model routing for oh-my-claudecode integration
  - group1 agents (explore, architect, researcher, critic, analyst) → `gemini-3-pro-preview`
  - group2 agents (executor, designer, etc.) → standard model mapping
- Thinking block stripping for non-Claude backends (Gemini compatibility)
- Agent detection from system prompt (`oh-my-claudecode:{agent-name}` pattern)
- Config validation with startup warnings for agent routing misconfigurations
- Comprehensive test suite for agent detection and classification (36 tests)

### Fixed

- 400 errors when Claude thinking blocks were sent to Gemini backends

## [4.0.0] - 2026-01-30

### Added

- Three routing modes: `antigravity`, `claude`, `auto`
- Bidirectional automatic failover state machine with cooldown escalation (30m -> 60m -> 120m -> 240m)
- fsnotify-based zero-downtime mode and config hot-reload
- Glob pattern matching for model name rewriting (`claude-sonnet-*` -> `gemini-claude-sonnet-4-5-thinking`)
- Switchable response writer for buffering error responses during auto-retry
- SSE streaming support via reverse proxy
- Health endpoint (`/health`) with auto-switch state details
- CLI commands: `serve`, `start`, `stop`, `restart`, `status`, `check`
- Mode switching commands with aliases: `antigravity` (`ag`), `claude` (`c`), `auto` (`a`)
- Config management: `config view`, `config edit`, `config reset`, `config path`
- Daemon management with PID file tracking
- Platform support for macOS (launchd) and Linux (systemd)
- Date-based log rotation (`~/.rrouter/logs/YYYY-MM-DD.log`)
- Graceful shutdown on SIGTERM/SIGINT
- Generation counter to prevent stale cooldown timer race conditions
- Cooldown decay: 2x cooldown period of healthy operation resets to initial
- Installation and uninstallation scripts
- Bilingual documentation (English/Korean)
- Embedded default configuration via Go embed
- Environment variables: `RROUTER_PORT`, `RROUTER_UPSTREAM`
