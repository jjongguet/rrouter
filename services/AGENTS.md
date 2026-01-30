<!-- Parent: ../AGENTS.md -->

# services

## Purpose

Contains systemd and launchd service configuration files for running rrouter as a background daemon on Linux and macOS systems.

## Key Files

| File | Purpose |
|------|---------|
| `rrouter-proxy.service` | systemd service unit file for Linux |
| `com.rrouter.plist` | launchd property list file for macOS |

## File Details

### systemd Service (`rrouter-proxy.service`)

**Target Platform**: Linux with systemd

**Installation Path**: `~/.config/systemd/user/rrouter.service`

**Configuration**:
- **Type**: `simple` (foreground process)
- **ExecStart**: `%h/.local/bin/rrouter-proxy` (legacy path, should be `rrouter serve`)
- **Restart**: `always` with 1-second delay
- **Environment**:
  - `RROUTER_UPSTREAM=http://localhost:8317`
  - `RROUTER_PORT=8316`
- **Working Directory**: User home directory
- **Logging**: `journal` (systemd journal)

**Security Hardening**:
- `NoNewPrivileges=true`: Prevents privilege escalation
- `PrivateTmp=true`: Isolated /tmp directory
- `ProtectSystem=strict`: Read-only system directories
- `ProtectHome=read-only`: Read-only home except allowed paths
- `ReadWritePaths=%h/.rrouter`: Write access to config directory

**Commands**:
```bash
# Enable and start
systemctl --user enable --now rrouter

# Status
systemctl --user status rrouter

# Restart
systemctl --user restart rrouter

# Logs
journalctl --user -u rrouter -f
```

### launchd Service (`com.rrouter.plist`)

**Target Platform**: macOS

**Installation Path**: `~/Library/LaunchAgents/com.rrouter.plist`

**Configuration**:
- **Label**: `com.rrouter`
- **ProgramArguments**: `__HOME__/.local/bin/rrouter serve` (placeholder replaced during install)
- **Environment Variables**:
  - `RROUTER_UPSTREAM=http://localhost:8317`
  - `RROUTER_PORT=8316`
- **Working Directory**: User home directory
- **RunAtLoad**: `true` (start on login)
- **KeepAlive**: `true` (restart on crash)
- **Error Log**: `__HOME__/.rrouter/logs/rrouter.error.log`
- **ProcessType**: `Background`

**Commands**:
```bash
# Load and start
launchctl load ~/Library/LaunchAgents/com.rrouter.plist

# Unload
launchctl unload ~/Library/LaunchAgents/com.rrouter.plist

# Restart
launchctl kickstart -k gui/$(id -u)/com.rrouter

# Check status
launchctl list | grep rrouter
```

## For AI Agents

When working with service files:

1. **Installation**: Both files use placeholder paths that are replaced by `install.sh`:
   - `__HOME__` → Actual user home directory
   - `%h` (systemd) → Expands to home at runtime

2. **Binary Path Discrepancy**:
   - systemd references `rrouter-proxy` (legacy name)
   - launchd references `rrouter serve` (correct)
   - Should standardize to `rrouter serve` for both

3. **Environment Variables**:
   - Both set `RROUTER_PORT=8316` and `RROUTER_UPSTREAM=http://localhost:8317`
   - Can be overridden by editing service files
   - Changes require daemon reload (systemd) or plist reload (launchd)

4. **Logging**:
   - systemd: Uses journal (`journalctl --user -u rrouter`)
   - launchd: Writes to `~/.rrouter/logs/rrouter.error.log`
   - Proxy also logs to `~/.rrouter/logs/YYYY-MM-DD.log` internally

5. **Auto-Restart**:
   - systemd: `Restart=always` with 1-second delay
   - launchd: `KeepAlive=true`
   - Both ensure proxy stays running after crashes

6. **User-Level Services**:
   - systemd: `systemctl --user` (user session)
   - launchd: `~/Library/LaunchAgents/` (per-user)
   - No root/sudo required

7. **Common Issues**:
   - Wrong binary path (legacy `rrouter-proxy` vs current `rrouter`)
   - Port conflicts (8316/8317 already in use)
   - Permission issues on `~/.rrouter/` directory
   - Environment variable precedence

8. **Testing Changes**:
   ```bash
   # systemd
   systemctl --user daemon-reload
   systemctl --user restart rrouter

   # launchd
   launchctl unload ~/Library/LaunchAgents/com.rrouter.plist
   launchctl load ~/Library/LaunchAgents/com.rrouter.plist
   ```

9. **Debugging**:
   - Verify binary exists at specified path
   - Check environment variables are set correctly
   - Review logs for startup errors
   - Ensure clipproxyapi is running on port 8317
