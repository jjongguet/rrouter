package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var (
	homeDir    string
	rrouterDir string
	pidFile    string
	modeFile   string
)

func init() {
	var err error
	homeDir, err = os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[rrouter] Cannot get home directory: %v\n", err)
		os.Exit(1)
	}
	rrouterDir = filepath.Join(homeDir, ".rrouter")
	pidFile = filepath.Join(rrouterDir, "rrouter.pid")
	modeFile = filepath.Join(rrouterDir, "mode")
}

// migratePIDFile handles migration from old rrouterd.pid to rrouter.pid
func migratePIDFile() {
	oldPidFile := filepath.Join(rrouterDir, "rrouterd.pid")
	if _, err := os.Stat(oldPidFile); err == nil {
		// Old PID file exists, check if it has a running process
		if data, err := os.ReadFile(oldPidFile); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
				if syscall.Kill(pid, 0) == nil {
					// Process still running under old name, stop it first
					fmt.Println("[rrouter] Stopping old rrouterd process...")
					syscall.Kill(pid, syscall.SIGTERM)
					time.Sleep(time.Second)
				}
			}
		}
		os.Remove(oldPidFile)
	}
}

// isRunning checks if the daemon is running using PID file + kill -0 (primary)
// and port check (fallback for launchd-started daemons).
func isRunning() bool {
	// PRIMARY: PID file + kill -0
	if data, err := os.ReadFile(pidFile); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			if syscall.Kill(pid, 0) == nil {
				return true
			}
		}
		// Stale PID file, remove it
		os.Remove(pidFile)
	}

	// FALLBACK: port check (for launchd-started daemons without PID file)
	if runtime.GOOS == "darwin" {
		cmd := exec.Command("lsof", "-i", ":8316", "-sTCP:LISTEN")
		if cmd.Run() == nil {
			return true
		}
	} else {
		cmd := exec.Command("ss", "-tlnp", "sport", "=", "8316")
		if cmd.Run() == nil {
			return true
		}
	}

	return false
}

// getPID returns the PID of the running daemon, or 0 if not running.
func getPID() int {
	if data, err := os.ReadFile(pidFile); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			if syscall.Kill(pid, 0) == nil {
				return pid
			}
		}
	}
	return 0
}

// cmdStart starts the daemon in the background.
func cmdStart() {
	if isRunning() {
		pid := getPID()
		if pid > 0 {
			fmt.Printf("[rrouter] Daemon already running (PID: %d)\n", pid)
		} else {
			fmt.Println("[rrouter] Daemon already running (via launchd or port 8316 in use)")
		}
		return
	}

	migratePIDFile()

	// Ensure directories exist
	logDir := filepath.Join(rrouterDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "[rrouter] Failed to create log directory: %v\n", err)
		os.Exit(1)
	}

	// Get executable path
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[rrouter] Failed to get executable path: %v\n", err)
		os.Exit(1)
	}

	// Open log file
	logPath := filepath.Join(logDir, "daemon.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[rrouter] Failed to open log file: %v\n", err)
		os.Exit(1)
	}

	// Start daemon process
	cmd := exec.Command(exe, "serve")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		logFile.Close()
		fmt.Fprintf(os.Stderr, "[rrouter] Failed to start daemon: %v\n", err)
		os.Exit(1)
	}

	// Write PID file
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "[rrouter] Warning: Failed to write PID file: %v\n", err)
	}

	// Release the process so it continues running after we exit
	cmd.Process.Release()
	logFile.Close()

	fmt.Printf("[rrouter] Daemon started (PID: %d)\n", cmd.Process.Pid)
	fmt.Printf("[rrouter] Log file: %s\n", logPath)
}

// cmdStop stops the running daemon.
func cmdStop() {
	if !isRunning() {
		fmt.Println("[rrouter] Daemon is not running")
		return
	}

	pid := getPID()
	if pid > 0 {
		fmt.Printf("[rrouter] Stopping daemon (PID: %d)...\n", pid)
		if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
			fmt.Fprintf(os.Stderr, "[rrouter] Failed to send SIGTERM: %v\n", err)
		}
	} else {
		// Fallback: try to find and kill by process name
		fmt.Println("[rrouter] Stopping daemon...")
		if runtime.GOOS == "darwin" {
			exec.Command("pkill", "-f", "rrouter serve").Run()
		} else {
			exec.Command("pkill", "-f", "rrouter serve").Run()
		}
	}

	// Wait for process to exit
	for i := 0; i < 10; i++ {
		time.Sleep(200 * time.Millisecond)
		if !isRunning() {
			break
		}
	}

	if isRunning() {
		fmt.Println("[rrouter] Daemon did not stop gracefully, sending SIGKILL...")
		if pid > 0 {
			syscall.Kill(pid, syscall.SIGKILL)
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Clean up PID file
	os.Remove(pidFile)

	if !isRunning() {
		fmt.Println("[rrouter] Daemon stopped")
	} else {
		fmt.Fprintf(os.Stderr, "[rrouter] Failed to stop daemon\n")
		os.Exit(1)
	}
}

// cmdRestart restarts the daemon.
func cmdRestart() {
	if isRunning() {
		cmdStop()
		time.Sleep(500 * time.Millisecond)
	}
	cmdStart()
}

// cmdStatus shows the current daemon and mode status.
func cmdStatus() {
	fmt.Println()
	fmt.Println("===========================================")
	fmt.Println("  rrouter Status")
	fmt.Println("===========================================")
	fmt.Println()

	// Mode status
	mode := getCurrentMode()
	if mode == "" {
		fmt.Println("  Mode:        Not set (will use default: claude)")
	} else {
		switch mode {
		case "antigravity":
			fmt.Println("  Mode:        Antigravity")
		case "claude":
			fmt.Println("  Mode:        Claude (OAuth passthrough)")
		case "auto":
			fmt.Println("  Mode:        Auto (Antigravity-first, Claude fallback)")
			// Try to get auto-switch status from health endpoint
			if isRunning() {
				showAutoSwitchStatus()
			}
		default:
			fmt.Printf("  Mode:        Unknown: %s\n", mode)
		}
	}
	fmt.Println()

	// Service status
	fmt.Println("Services:")

	// rrouter status with port
	if isRunning() {
		pid := getPID()
		if pid > 0 {
			fmt.Printf("  rrouter (:%d):       Running (PID: %d)\n", 8316, pid)
		} else {
			fmt.Printf("  rrouter (:%d):       Running (via launchd)\n", 8316)
		}
	} else {
		fmt.Printf("  rrouter (:%d):       Not running\n", 8316)
	}

	// cliproxyapi status with port
	if isCliproxyapiRunning() {
		fmt.Printf("  cliproxyapi (:%d):   Running\n", 8317)
	} else {
		fmt.Printf("  cliproxyapi (:%d):   Not running\n", 8317)
	}

	fmt.Println()
	fmt.Println("===========================================")
	fmt.Println()
}

// getCurrentMode reads the current mode from the mode file.
func getCurrentMode() string {
	data, err := os.ReadFile(modeFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// isAntigravityProxyRunning checks if antigravity-proxy is running.
func isAntigravityProxyRunning() bool {
	cmd := exec.Command("pgrep", "-f", "antigravity-proxy")
	return cmd.Run() == nil
}

// isCliproxyapiRunning checks if cliproxyapi is running by checking port 8317.
func isCliproxyapiRunning() bool {
	conn, err := net.DialTimeout("tcp", "localhost:8317", 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// showAutoSwitchStatus fetches and displays auto-switch status from /health.
func showAutoSwitchStatus() {
	resp, err := http.Get("http://localhost:8316/health")
	if err != nil {
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	bodyStr := string(body)

	// Extract fields
	currentTarget := extractJSONString(bodyStr, "currentTarget")
	defaultTarget := extractJSONString(bodyStr, "defaultTarget")
	autoSwitched := extractJSONBool(bodyStr, "autoSwitched")
	switchCount := extractJSONInt(bodyStr, "autoSwitchCount")

	fmt.Println()
	fmt.Println("  Auto-Switch Status:")
	fmt.Printf("    Default target:    %s\n", defaultTarget)

	if autoSwitched {
		fmt.Printf("    Currently routing: %s (switched from %s)\n", currentTarget, defaultTarget)
		if switchCount > 0 {
			fmt.Printf("    Switches so far:   %d\n", switchCount)
		}
		cooldownRemaining := extractJSONString(bodyStr, "cooldownRemaining")
		if cooldownRemaining != "" {
			fmt.Printf("    Cooldown remaining: %s (will retry %s)\n", cooldownRemaining, defaultTarget)
		}
	} else {
		fmt.Printf("    Currently routing: %s\n", currentTarget)
		if switchCount > 0 {
			fmt.Printf("    Total switches:    %d (recovered)\n", switchCount)
		}
	}
}

// Simple JSON field extractors (avoiding full JSON parsing for status display)
func extractJSONString(json, field string) string {
	key := fmt.Sprintf(`"%s":"`, field)
	start := strings.Index(json, key)
	if start == -1 {
		return ""
	}
	start += len(key)
	end := strings.Index(json[start:], `"`)
	if end == -1 {
		return ""
	}
	return json[start : start+end]
}

func extractJSONBool(json, field string) bool {
	key := fmt.Sprintf(`"%s":`, field)
	start := strings.Index(json, key)
	if start == -1 {
		return false
	}
	start += len(key)
	return strings.HasPrefix(json[start:], "true")
}

func extractJSONInt(json, field string) int {
	key := fmt.Sprintf(`"%s":`, field)
	start := strings.Index(json, key)
	if start == -1 {
		return 0
	}
	start += len(key)
	// Find end of number
	end := start
	for end < len(json) && json[end] >= '0' && json[end] <= '9' {
		end++
	}
	if end == start {
		return 0
	}
	val, _ := strconv.Atoi(json[start:end])
	return val
}
