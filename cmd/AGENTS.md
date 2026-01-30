<!-- Parent: ../AGENTS.md -->

# cmd

## Purpose

Contains subdirectories with main package source code for the rrouter project. Currently houses the `rrouter` command which provides both CLI utilities and the HTTP proxy server in a single binary.

## Key Files

None - this directory only contains subdirectories.

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| `rrouter/` | Main Go source code for the rrouter CLI and proxy server |

## For AI Agents

This is an organizational directory following Go conventions where `cmd/` contains command-line applications. Each subdirectory under `cmd/` represents a separate executable binary.

For the rrouter project:
- `cmd/rrouter/` builds to a single `rrouter` binary
- The binary combines CLI management tools and the HTTP proxy server
- Use `go build -o rrouter ./cmd/rrouter` to build
