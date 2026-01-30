package main

import "fmt"

func cmdHelp() {
	fmt.Printf(`
rrouter v%s - Unified CLI and daemon for API routing

USAGE:
  rrouter <command> [options]

MODE COMMANDS:
  antigravity, ag     Switch to Antigravity mode
  claude, c           Switch to Claude OAuth passthrough
  auto, a             Switch to Auto mode (AG-first, Claude fallback)

DAEMON COMMANDS:
  serve               Run daemon in foreground (used internally)
  start               Start daemon in background
  stop                Stop running daemon
  restart             Restart daemon
  status              Show current mode and daemon status

CONFIG COMMANDS:
  config              View current config.json
  config edit         Edit config.json in $EDITOR
  config reset        Reset config.json to defaults
  config path         Show config file path

OTHER COMMANDS:
  health, --check     Run health check
  help, --help, -h    Show this help message
  version, -v         Show version

MODES:
  antigravity    Route through Antigravity proxy with model rewriting
  claude         Direct Claude OAuth passthrough (no rewriting)
  auto           Antigravity-first with automatic Claude fallback on errors

EXAMPLES:
  rrouter ag            # Switch to Antigravity mode
  rrouter claude        # Switch to Claude passthrough
  rrouter status        # Check current mode and daemon
  rrouter start         # Start the daemon
  rrouter --check       # Run health check

FILES:
  ~/.rrouter/mode         Current mode setting
  ~/.rrouter/config.json  Model rewriting rules
  ~/.rrouter/rrouter.pid  Daemon PID file
  ~/.rrouter/logs/        Log files

`, Version)
}

func cmdVersion() {
	fmt.Printf("rrouter v%s\n", Version)
}
