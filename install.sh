#!/bin/bash
set -euo pipefail

# Print messages (no colors)
info() { echo "[INFO] $*"; }
warn() { echo "[WARN] $*"; }
error() { echo "[ERROR] $*" >&2; }
dry() { echo "[DRY-RUN] $*"; }

# Parse arguments
DRY_RUN=false
for arg in "$@"; do
    case "$arg" in
        --dry-run|-n)
            DRY_RUN=true
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --dry-run, -n    Show what would be done without making changes"
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

# Wrapper for commands (execute or print based on DRY_RUN)
run() {
    if [[ "$DRY_RUN" == true ]]; then
        dry "Would run: $*"
    else
        "$@"
    fi
}

# Check if running as root
if [[ $EUID -eq 0 ]]; then
   error "This script should NOT be run as root"
   exit 1
fi

if [[ "$DRY_RUN" == true ]]; then
    echo ""
    warn "=== DRY-RUN MODE: No changes will be made ==="
    echo ""
fi

# Get script directory
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$SCRIPT_DIR"

# Installation paths
INSTALL_DIR="$HOME/.local/bin"
CONFIG_DIR="$HOME/.rrouter"
LOG_DIR="$CONFIG_DIR/logs"
CLAUDE_SETTINGS="$HOME/.claude/settings.json"

# Step 1: Build Go binary
info "Building rrouter..."
if ! command -v go &> /dev/null; then
    error "Go is not installed. Please install Go 1.22 or later."
    exit 1
fi

cd "$PROJECT_ROOT"
if [[ "$DRY_RUN" == true ]]; then
    dry "Would copy config.json to cmd/rrouter/ for embed"
    dry "Would run: go build -o rrouter-bin ./cmd/rrouter"
else
    # Copy root config.json into cmd/rrouter/ for Go embed
    cp "$PROJECT_ROOT/config.json" "$PROJECT_ROOT/cmd/rrouter/config.json" 2>/dev/null || true
    if ! go build -o rrouter-bin ./cmd/rrouter; then
        error "Failed to build rrouter"
        exit 1
    fi
fi

if [[ "$DRY_RUN" != true ]]; then
    info "Build successful"
fi

# Step 2: Create installation directories
info "Creating installation directories..."
run mkdir -p "$INSTALL_DIR"
run mkdir -p "$CONFIG_DIR"
run mkdir -p "$LOG_DIR"

# Step 3: Install rrouter binary
info "Installing rrouter to $INSTALL_DIR..."

if [[ "$DRY_RUN" == true ]]; then
    dry "Would run: install -m 755 rrouter-bin $INSTALL_DIR/rrouter"
else
    if ! install -m 755 rrouter-bin "$INSTALL_DIR/rrouter"; then
        error "Failed to install rrouter"
        exit 1
    fi
    rm -f rrouter-bin  # Clean up build artifact
fi

# Step 4: Set default mode and copy example config
if [[ ! -f "$CONFIG_DIR/mode" ]] || [[ "$DRY_RUN" == true ]]; then
    info "Setting default mode to 'auto'..."
    if [[ "$DRY_RUN" == true ]]; then
        if [[ -f "$CONFIG_DIR/mode" ]]; then
            dry "Mode file already exists, would keep current mode"
        else
            dry "Would write 'auto' to $CONFIG_DIR/mode"
        fi
    else
        echo "auto" > "$CONFIG_DIR/mode"
        info "Default mode set to 'auto'"
    fi
else
    info "Mode file exists, keeping current mode: $(cat "$CONFIG_DIR/mode")"
fi

# Copy example config if config.json doesn't exist
if [[ ! -f "$CONFIG_DIR/config.json" ]] || [[ "$DRY_RUN" == true ]]; then
    if [[ -f "$PROJECT_ROOT/.rrouter-config.example.json" ]]; then
        if [[ "$DRY_RUN" == true ]]; then
            dry "Would copy .rrouter-config.example.json to $CONFIG_DIR/config.json"
        else
            cp "$PROJECT_ROOT/.rrouter-config.example.json" "$CONFIG_DIR/config.json"
            info "Created default config.json"
        fi
    fi
fi

# Step 5: Start the daemon
info "Starting rrouter daemon..."
if [[ "$DRY_RUN" == true ]]; then
    dry "Would run: $INSTALL_DIR/rrouter start"
else
    if "$INSTALL_DIR/rrouter" start; then
        info "rrouter daemon is running"
    else
        warn "Failed to start rrouter daemon automatically"
        warn "Run 'rrouter start' manually after installation"
    fi
fi

# Step 6: Update Claude Code settings
info "Updating Claude Code settings..."
if [[ "$DRY_RUN" == true ]]; then
    dry "Would create $(dirname "$CLAUDE_SETTINGS") if not exists"
    if [[ -f "$CLAUDE_SETTINGS" ]]; then
        dry "Would backup $CLAUDE_SETTINGS"
        dry "Would set env.ANTHROPIC_BASE_URL = http://localhost:8316 using jq"
    else
        dry "Would create $CLAUDE_SETTINGS with env.ANTHROPIC_BASE_URL"
    fi
else
    mkdir -p "$(dirname "$CLAUDE_SETTINGS")"

    if [[ -f "$CLAUDE_SETTINGS" ]]; then
        # Backup existing settings
        cp "$CLAUDE_SETTINGS" "$CLAUDE_SETTINGS.backup.$(date +%Y%m%d_%H%M%S)"
        info "Backed up existing settings"

        # Update using jq if available
        if command -v jq &> /dev/null; then
            TEMP_FILE=$(mktemp "$CLAUDE_SETTINGS.XXXXXX")
            # Remove old anthropicProxyPort if exists, add env.ANTHROPIC_BASE_URL
            jq 'del(.anthropicProxyPort) | .env.ANTHROPIC_BASE_URL = "http://localhost:8316"' "$CLAUDE_SETTINGS" > "$TEMP_FILE"
            mv "$TEMP_FILE" "$CLAUDE_SETTINGS"
            info "Updated ANTHROPIC_BASE_URL to http://localhost:8316 in $CLAUDE_SETTINGS"
        else
            warn "jq not found - please manually set env.ANTHROPIC_BASE_URL in $CLAUDE_SETTINGS"
        fi
    else
        # Create new settings file
        cat > "$CLAUDE_SETTINGS" <<EOF
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://localhost:8316"
  }
}
EOF
        info "Created new Claude Code settings"
    fi
fi

# Step 7: Check PATH
info "Checking PATH configuration..."
if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
    warn "$INSTALL_DIR is not in your PATH"
    warn "Add this line to your shell profile (~/.bashrc, ~/.zshrc, etc.):"
    echo ""
    echo "    export PATH=\"\$HOME/.local/bin:\$PATH\""
    echo ""
fi

# Step 8: Installation summary
echo ""
if [[ "$DRY_RUN" == true ]]; then
    warn "=== DRY-RUN COMPLETE: No changes were made ==="
    echo ""
    echo "To perform actual installation, run without --dry-run:"
    echo "  ./install.sh"
else
    info "Installation complete!"
fi
echo ""
echo "Installation summary:"
echo "  - rrouter installed to: $INSTALL_DIR/rrouter"
echo "  - Configuration directory: $CONFIG_DIR"
echo "  - Log directory: $LOG_DIR"
echo "  - Daemon: started via 'rrouter start'"
echo "  - Default mode: auto"
echo "  - Claude Code base URL: http://localhost:8316"
echo ""
echo "Quick start:"
echo "  rrouter status              # Check status"
echo "  rrouter antigravity         # Switch to antigravity mode"
echo "  rrouter claude              # Switch to claude mode"
echo "  rrouter auto                # Switch to auto mode"
echo "  rrouter config              # View config"
echo ""
echo "Daemon management:"
echo "  rrouter start               # Start daemon"
echo "  rrouter stop                # Stop daemon"
echo "  rrouter restart             # Restart daemon"
echo "  rrouter status              # Check status"

echo ""
echo "Log files: $LOG_DIR/YYYY-MM-DD.log"
echo ""
if [[ "$DRY_RUN" != true ]]; then
    info "New Claude Code sessions will use the proxy (existing sessions may need restart)"
fi
