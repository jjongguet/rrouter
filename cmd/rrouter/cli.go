package main

import (
	"fmt"
	"os"
)

const Version = "4.0.0"

func main() {
	if len(os.Args) < 2 {
		cmdHelp()
		os.Exit(0)
	}

	switch os.Args[1] {
	case "serve":
		cmdServe()
	case "start":
		cmdStart()
	case "stop":
		cmdStop()
	case "restart":
		cmdRestart()
	case "status":
		cmdStatus()
	case "antigravity", "ag":
		cmdAntigravity()
	case "claude", "c":
		cmdClaude()
	case "auto", "a":
		cmdAuto()
	case "config":
		cmdConfig(os.Args[2:])
	case "health", "--check", "check":
		cmdHealth()
	case "help", "--help", "-h":
		cmdHelp()
	case "version", "--version", "-v":
		cmdVersion()
	default:
		fmt.Fprintf(os.Stderr, "[rrouter] Unknown command: %s\n\n", os.Args[1])
		cmdHelp()
		os.Exit(1)
	}
}
