package main

import (
	"fmt"
	"os"
	"runtime"
)

// setMode writes the mode to the mode file.
func setMode(mode string) error {
	// Ensure directory exists
	if err := os.MkdirAll(rrouterDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	if err := os.WriteFile(modeFile, []byte(mode), 0644); err != nil {
		return fmt.Errorf("failed to write mode file: %w", err)
	}
	return nil
}

// restartProxyHint returns platform-appropriate hint for restarting the proxy.
func restartProxyHint() string {
	if runtime.GOOS == "darwin" {
		return fmt.Sprintf("launchctl kickstart -k gui/%d/com.rrouter", os.Getuid())
	}
	return "systemctl --user restart rrouter"
}

// antigravityProxyHint returns platform-appropriate hint for starting antigravity-proxy.
func antigravityProxyHint() string {
	if runtime.GOOS == "darwin" {
		return fmt.Sprintf("launchctl kickstart -k gui/%d/com.antigravity-proxy", os.Getuid())
	}
	return "systemctl --user restart antigravity-proxy"
}

// warnIfDaemonNotRunning prints a warning if the daemon is not running.
func warnIfDaemonNotRunning() {
	if !isRunning() {
		fmt.Println()
		fmt.Println("[rrouter] Warning: Daemon is not running!")
		fmt.Printf("[rrouter] Start it with: rrouter start (or %s)\n", restartProxyHint())
	}
}

// warnIfAntigravityNotRunning prints a warning if antigravity-proxy is not running.
func warnIfAntigravityNotRunning() {
	if !isAntigravityProxyRunning() {
		fmt.Println()
		fmt.Println("[rrouter] Warning: antigravity-proxy is not running!")
		fmt.Printf("[rrouter] Start it with: %s\n", antigravityProxyHint())
	}
}

// cmdAntigravity switches to Antigravity mode.
func cmdAntigravity() {
	previousMode := getCurrentMode()
	if previousMode == "" {
		previousMode = "(not set)"
	}

	fmt.Println()
	fmt.Printf("[rrouter] Switching mode: %s -> antigravity\n", previousMode)

	if err := setMode("antigravity"); err != nil {
		fmt.Fprintf(os.Stderr, "[rrouter] Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[rrouter] Mode set to: antigravity")
	fmt.Println()
	fmt.Println("  Antigravity mode:")
	fmt.Println("  - All requests routed through Antigravity")
	fmt.Println("  - Model names rewritten (claude-* -> gemini-*)")
	fmt.Println("  - No automatic fallback")

	warnIfDaemonNotRunning()
	warnIfAntigravityNotRunning()
	fmt.Println()
}

// cmdClaude switches to Claude passthrough mode.
func cmdClaude() {
	previousMode := getCurrentMode()
	if previousMode == "" {
		previousMode = "(not set)"
	}

	fmt.Println()
	fmt.Printf("[rrouter] Switching mode: %s -> claude\n", previousMode)

	if err := setMode("claude"); err != nil {
		fmt.Fprintf(os.Stderr, "[rrouter] Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[rrouter] Mode set to: claude")
	fmt.Println()
	fmt.Println("  Claude mode:")
	fmt.Println("  - All requests passed through unchanged")
	fmt.Println("  - No model rewriting")
	fmt.Println("  - No automatic fallback")

	warnIfDaemonNotRunning()
	fmt.Println()
}

// cmdAuto switches to Auto mode.
func cmdAuto() {
	previousMode := getCurrentMode()
	if previousMode == "" {
		previousMode = "(not set)"
	}

	fmt.Println()
	fmt.Printf("[rrouter] Switching mode: %s -> auto\n", previousMode)

	if err := setMode("auto"); err != nil {
		fmt.Fprintf(os.Stderr, "[rrouter] Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[rrouter] Mode set to: auto")
	fmt.Println()
	fmt.Println("  Auto mode (bidirectional failover):")
	fmt.Println("  - Starts with Antigravity as default target")
	fmt.Println("  - On failure (429/5xx x3 or timeout x2): switches to the OTHER target")
	fmt.Println("  - After cooldown (30m-4h): retries the failed target")
	fmt.Println("  - Works BOTH ways: Antigravity <-> Claude")
	fmt.Println("  - Any successful response resets the failure counter")

	warnIfDaemonNotRunning()
	warnIfAntigravityNotRunning()
	fmt.Println()
}
