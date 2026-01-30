package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var configFile string

func init() {
	// Note: homeDir and rrouterDir are already initialized in daemon.go
	// We initialize configFile here after they're set
}

func getConfigFile() string {
	if configFile == "" {
		configFile = filepath.Join(rrouterDir, "config.json")
	}
	return configFile
}

// getDefaultConfig returns the default configuration JSON.
func getDefaultConfig() string {
	return `{
  "modes": {
    "antigravity": {
      "mappings": [
        { "match": "claude-sonnet-*", "rewrite": "gemini-claude-sonnet-4-5-thinking" },
        { "match": "claude-opus-*", "rewrite": "gemini-claude-opus-4-5-thinking" },
        { "match": "claude-haiku-*", "rewrite": "gemini-3-flash-preview" }
      ]
    },
    "claude": {
      "mappings": []
    }
  },
  "defaultMode": "claude"
}`
}

// cmdConfig handles config subcommands.
func cmdConfig(args []string) {
	if len(args) == 0 {
		cmdConfigShow()
		return
	}

	switch args[0] {
	case "edit":
		cmdConfigEdit()
	case "reset":
		cmdConfigReset()
	case "path":
		cmdConfigPath()
	default:
		fmt.Fprintf(os.Stderr, "[rrouter] Unknown config subcommand: %s\n", args[0])
		fmt.Println()
		fmt.Println("Available config commands:")
		fmt.Println("  rrouter config        View current config")
		fmt.Println("  rrouter config edit   Edit config in $EDITOR")
		fmt.Println("  rrouter config reset  Reset to defaults")
		fmt.Println("  rrouter config path   Show config file path")
		os.Exit(1)
	}
}

// cmdConfigShow displays the current configuration.
func cmdConfigShow() {
	cf := getConfigFile()
	data, err := os.ReadFile(cf)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("[rrouter] No config file found at: %s\n", cf)
			fmt.Println("[rrouter] Run 'rrouter config reset' to create one with defaults.")
			return
		}
		fmt.Fprintf(os.Stderr, "[rrouter] Error reading config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[rrouter] Current config at: %s\n", cf)
	fmt.Println()
	fmt.Println(string(data))
}

// cmdConfigEdit opens the config file in the user's editor.
func cmdConfigEdit() {
	cf := getConfigFile()

	// Create config with defaults if it doesn't exist
	if _, err := os.Stat(cf); os.IsNotExist(err) {
		fmt.Println("[rrouter] Config file not found. Creating with defaults...")
		if err := os.MkdirAll(rrouterDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "[rrouter] Failed to create directory: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(cf, []byte(getDefaultConfig()), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "[rrouter] Failed to create config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[rrouter] Created config file at: %s\n", cf)
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	fmt.Printf("[rrouter] Opening config in %s...\n", editor)

	cmd := exec.Command(editor, cf)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "[rrouter] Editor exited with error: %v\n", err)
		os.Exit(1)
	}
}

// cmdConfigReset resets the config to defaults.
func cmdConfigReset() {
	cf := getConfigFile()

	// Check if config already exists
	if _, err := os.Stat(cf); err == nil {
		fmt.Printf("[rrouter] Config file already exists at: %s\n", cf)
		fmt.Print("Overwrite with defaults? (y/N): ")

		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		if input != "y" && input != "yes" {
			fmt.Println("[rrouter] Cancelled. Config file unchanged.")
			return
		}
	}

	// Ensure directory exists
	if err := os.MkdirAll(rrouterDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "[rrouter] Failed to create directory: %v\n", err)
		os.Exit(1)
	}

	// Write default config
	if err := os.WriteFile(cf, []byte(getDefaultConfig()), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "[rrouter] Failed to write config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[rrouter] Config reset to defaults at: %s\n", cf)
}

// cmdConfigPath prints the config file path.
func cmdConfigPath() {
	fmt.Println(getConfigFile())
}
