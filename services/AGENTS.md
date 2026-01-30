<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-01-30 | Updated: 2026-01-30 -->

# services

## Purpose

System service definition files for running rrouter as a background daemon on macOS (launchd) and Linux (systemd). These are optional - the preferred method is `rrouter start` which manages the daemon directly.

## Key Files

| File | Description |
|------|-------------|
| `com.rrouter.plist` | macOS launchd service definition |
| `rrouter-proxy.service` | Linux systemd user service definition (legacy name) |

## For AI Agents

### Working In This Directory

- These files are **templates** - not auto-installed by `install.sh`
- The `rrouter start` command is the primary daemon management method
- These files exist for users who prefer system service managers

### File Purposes

**com.rrouter.plist** (macOS):
- Install to: `~/Library/LaunchAgents/`
- Load with: `launchctl load ~/Library/LaunchAgents/com.rrouter.plist`
- Has `KeepAlive: true` - launchd will restart if it dies

**rrouter-proxy.service** (Linux):
- Install to: `~/.config/systemd/user/`
- Enable with: `systemctl --user enable rrouter-proxy`
- Note: Filename still uses old `rrouter-proxy` name for backward compatibility

### Important Notes

- The systemd service file references `rrouter-proxy` (legacy) - should be updated to use `rrouter serve`
- uninstall.sh handles cleanup of both old and new service file names
- Environment variables: `RROUTER_PORT` (default 8316), `RROUTER_UPSTREAM` (default http://localhost:8317)

<!-- MANUAL: -->
