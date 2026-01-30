#!/bin/bash
set -euo pipefail

# Print messages (no colors)
info() { echo "[INFO] $*"; }
warn() { echo "[WARN] $*"; }
error() { echo "[ERROR] $*" >&2; }
dry() { echo "[DRY-RUN] $*"; }

# Check if running as root
if [[ $EUID -eq 0 ]]; then
   error "This script should NOT be run as root"
   exit 1
fi

# Parse arguments
DRY_RUN=false
PURGE=false
for arg in "$@"; do
    case "$arg" in
        --dry-run|-n)
            DRY_RUN=true
            ;;
        --purge)
            PURGE=true
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --dry-run, -n    Show what would be done without making changes"
            echo "  --purge          Also remove config directory (~/.rrouter)"
            echo "  --help, -h       Show this help message"
            exit 0
            ;;
        *)
            echo "[ERROR] Unknown option: $arg" >&2
            echo "Run '$0 --help' for usage." >&2
            exit 1
            ;;
    esac
done


# Paths (must match install.sh)
INSTALL_DIR="$HOME/.local/bin"
CONFIG_DIR="$HOME/.rrouter"
CLAUDE_SETTINGS="$HOME/.claude/settings.json"

echo ""
echo "================================================="
echo "  rrouter Uninstaller"
echo "================================================="
echo ""

if [[ "$DRY_RUN" == true ]]; then
    warn "=== DRY-RUN MODE: No changes will be made ==="
    echo ""
fi

echo "This will remove:"
echo "  - rrouter binary from $INSTALL_DIR"
echo "  - Running daemon process"
echo "  - Claude Code ANTHROPIC_BASE_URL setting"
echo ""

if [[ "$PURGE" == true ]]; then
    warn "PURGE mode: will also remove $CONFIG_DIR"
else
    echo "NOTE: Configuration directory ($CONFIG_DIR) will be preserved."
    echo "      Use --purge to also remove config, mode file, and logs."
fi
echo ""

# Confirm (skip in dry-run mode)
if [[ "$DRY_RUN" != true ]]; then
    read -r -p "Continue with uninstall? [y/N] " response
    if [[ ! "$response" =~ ^[Yy]$ ]]; then
        info "Uninstall cancelled."
        exit 0
    fi
fi

echo ""

# Step 1: Stop rrouter daemon (using PID file)
PID_FILE="$CONFIG_DIR/rrouter.pid"
OLD_PID_FILE="$CONFIG_DIR/rrouterd.pid"

if [[ -f "$PID_FILE" ]]; then
    if [[ "$DRY_RUN" == true ]]; then
        dry "Would kill process from PID file: $PID_FILE"
    else
        info "Stopping rrouter daemon..."
        PID=$(cat "$PID_FILE" 2>/dev/null || echo "")
        if [[ -n "$PID" ]] && kill -0 "$PID" 2>/dev/null; then
            kill "$PID" 2>/dev/null || true
            sleep 1
        fi
    fi
fi

# Also stop old processes by name for backward compatibility
for proc in rrouterd rrouter-proxy; do
    if pgrep -x "$proc" > /dev/null 2>&1; then
        if [[ "$DRY_RUN" == true ]]; then
            dry "Would run: pkill -x $proc"
        else
            info "Stopping old $proc process..."
            pkill -x "$proc" 2>/dev/null || true
        fi
    fi
done

# Remove PID files
for pid_file in "$PID_FILE" "$OLD_PID_FILE"; do
    if [[ -f "$pid_file" ]]; then
        if [[ "$DRY_RUN" == true ]]; then
            dry "Would remove: $pid_file"
        else
            rm -f "$pid_file"
        fi
    fi
done

# Remove any launchd/systemd service files that may exist
for f in "$HOME/Library/LaunchAgents/com.rrouter.plist" "$HOME/Library/LaunchAgents/com.rrouter-proxy.plist"; do
    if [[ -f "$f" ]]; then
        if [[ "$DRY_RUN" == true ]]; then
            dry "Would unload and remove launchd service: $f"
        else
            info "Unloading launchd service: $f"
            launchctl bootout "gui/$(id -u)" "$f" 2>/dev/null || launchctl unload "$f" 2>/dev/null || true
            rm -f "$f"
        fi
    fi
done
for f in "$HOME/.config/systemd/user/rrouter.service" "$HOME/.config/systemd/user/rrouter-proxy.service"; do
    if [[ -f "$f" ]]; then
        if [[ "$DRY_RUN" == true ]]; then
            dry "Would disable and remove systemd service: $f"
        else
            svc_name=$(basename "$f")
            info "Disabling systemd service: $svc_name"
            systemctl --user stop "$svc_name" 2>/dev/null || true
            systemctl --user disable "$svc_name" 2>/dev/null || true
            rm -f "$f"
        fi
    fi
done

info "Process and service cleanup complete"

# Step 2: Remove binaries
for bin in rrouterd rrouter rrouter-proxy rrouter-cli; do
    if [[ -f "$INSTALL_DIR/$bin" ]]; then
        if [[ "$DRY_RUN" == true ]]; then
            dry "Would remove: $INSTALL_DIR/$bin"
        else
            info "Removing $bin..."
            rm -f "$INSTALL_DIR/$bin"
        fi
    fi
done

info "Binaries removed"

# Step 3: Remove Claude Code settings
if [[ -f "$CLAUDE_SETTINGS" ]]; then
    if command -v jq &> /dev/null; then
        # Remove both old (anthropicProxyPort) and new (env.ANTHROPIC_BASE_URL) settings
        HAS_OLD=$(jq -e '.anthropicProxyPort' "$CLAUDE_SETTINGS" 2>/dev/null && echo yes || echo no)
        HAS_NEW=$(jq -e '.env.ANTHROPIC_BASE_URL' "$CLAUDE_SETTINGS" 2>/dev/null && echo yes || echo no)

        if [[ "$HAS_OLD" == "yes" ]] || [[ "$HAS_NEW" == "yes" ]]; then
            if [[ "$DRY_RUN" == true ]]; then
                dry "Would backup: $CLAUDE_SETTINGS"
                dry "Would remove rrouter settings from Claude Code settings"
            else
                cp "$CLAUDE_SETTINGS" "$CLAUDE_SETTINGS.backup.$(date +%Y%m%d_%H%M%S)"
                info "Backed up Claude Code settings"
                info "Removing rrouter settings from Claude Code..."
                TEMP_FILE=$(mktemp)
                jq 'del(.anthropicProxyPort) | del(.env.ANTHROPIC_BASE_URL) | if .env == {} then del(.env) else . end' "$CLAUDE_SETTINGS" > "$TEMP_FILE"
                mv "$TEMP_FILE" "$CLAUDE_SETTINGS"
                info "Claude Code settings updated"
            fi
        fi
    else
        warn "jq not found - please manually remove ANTHROPIC_BASE_URL from $CLAUDE_SETTINGS"
    fi
fi

# Step 4: Purge config (optional)
if [[ "$PURGE" == true ]]; then
    if [[ -d "$CONFIG_DIR" ]]; then
        if [[ "$DRY_RUN" == true ]]; then
            dry "Would remove directory: $CONFIG_DIR"
        else
            info "Purging configuration directory: $CONFIG_DIR"
            rm -rf "$CONFIG_DIR"
            info "Configuration purged"
        fi
    fi
else
    if [[ -d "$CONFIG_DIR" ]]; then
        info "Configuration preserved at: $CONFIG_DIR"
        info "  To remove: rm -rf $CONFIG_DIR"
    fi
fi

# Step 5: Clean up build artifacts in project directory
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
for artifact in rrouter-proxy rrouter-bin rrouterd; do
    if [[ -f "$SCRIPT_DIR/$artifact" ]]; then
        if [[ "$DRY_RUN" == true ]]; then
            dry "Would remove build artifact: $SCRIPT_DIR/$artifact"
        else
            info "Removing build artifact: $SCRIPT_DIR/$artifact"
            rm -f "$SCRIPT_DIR/$artifact"
        fi
    fi
done

# Summary
echo ""
echo "================================================="
if [[ "$DRY_RUN" == true ]]; then
    echo "  DRY-RUN complete! No changes were made."
    echo "================================================="
    echo ""
    echo "To perform actual uninstall, run without --dry-run:"
    if [[ "$PURGE" == true ]]; then
        echo "  ./uninstall.sh --purge"
    else
        echo "  ./uninstall.sh"
    fi
else
    echo "  Uninstall complete!"
    echo "================================================="
    echo ""
    echo "Removed:"
    echo "  - rrouter binary"
    echo "  - Daemon process"
    echo "  - Claude Code ANTHROPIC_BASE_URL setting"
    if [[ "$PURGE" == true ]]; then
        echo "  - Configuration directory ($CONFIG_DIR)"
    fi
fi
echo ""
info "Please restart Claude Code to apply settings changes."
