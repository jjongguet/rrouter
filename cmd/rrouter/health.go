package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// cmdHealth runs a comprehensive health check.
func cmdHealth() {
	fmt.Println("[rrouter] Running health check...")
	fmt.Println()

	allOk := true

	// Check daemon status
	if isRunning() {
		pid := getPID()
		if pid > 0 {
			fmt.Printf("[OK]   rrouter daemon is running (PID: %d)\n", pid)
		} else {
			fmt.Println("[OK]   rrouter daemon is running (via launchd)")
		}
	} else {
		fmt.Println("[FAIL] rrouter daemon is NOT running")
		allOk = false
	}

	// Check antigravity-proxy status
	if isAntigravityProxyRunning() {
		fmt.Println("[OK]   antigravity-proxy is running")
	} else {
		fmt.Println("[--]   antigravity-proxy is not running (only needed for Antigravity mode)")
	}

	fmt.Println()

	// Check mode file
	mode := getCurrentMode()
	if mode != "" {
		fmt.Printf("[OK]   Mode file exists: %s\n", mode)
	} else {
		fmt.Println("[--]   Mode file not set (will use default: claude)")
	}

	fmt.Println()

	// Check health endpoint if daemon is running
	if isRunning() {
		fmt.Println("[rrouter] Checking health endpoint...")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get("http://localhost:8316/health")
		if err != nil {
			fmt.Printf("[FAIL] Cannot reach health endpoint: %v\n", err)
			allOk = false
		} else {
			defer resp.Body.Close()

			if resp.StatusCode == 200 {
				fmt.Println("[OK]   Health endpoint responded successfully")

				// Parse and display health info
				body, err := io.ReadAll(resp.Body)
				if err == nil {
					var health map[string]interface{}
					if json.Unmarshal(body, &health) == nil {
						// Display relevant health info
						if status, ok := health["status"].(string); ok {
							fmt.Printf("       Status: %s\n", status)
						}
						if reqCount, ok := health["requestCount"].(float64); ok {
							fmt.Printf("       Requests served: %.0f\n", reqCount)
						}
						if currentTarget, ok := health["currentTarget"].(string); ok {
							fmt.Printf("       Current target: %s\n", currentTarget)
						}
						if defaultMode, ok := health["defaultMode"].(string); ok {
							fmt.Printf("       Default mode: %s\n", defaultMode)
						}

						// Auto-switch info
						if autoSwitched, ok := health["autoSwitched"].(bool); ok && autoSwitched {
							fmt.Println()
							fmt.Println("       Auto-Switch Active:")
							if switchCount, ok := health["autoSwitchCount"].(float64); ok {
								fmt.Printf("         Switch count: %.0f\n", switchCount)
							}
							if cooldown, ok := health["cooldownRemaining"].(string); ok {
								fmt.Printf("         Cooldown remaining: %s\n", cooldown)
							}
						}
					}
				}
			} else {
				fmt.Printf("[WARN] Health endpoint returned: %d\n", resp.StatusCode)
			}
		}
	} else {
		fmt.Println("[--]   Skipping health endpoint check (daemon not running)")
	}

	fmt.Println()

	if allOk {
		fmt.Println("[rrouter] Health check complete!")
	} else {
		fmt.Println("[rrouter] Some services are not running. See above for details.")
		os.Exit(1)
	}
}
