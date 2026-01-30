<!-- Parent: ../AGENTS.md -->

# docs

## Purpose

Comprehensive technical documentation for the rrouter project, including complete guides, architecture details, and troubleshooting information.

## Key Files

| File | Purpose |
|------|---------|
| `complete-guide.md` | Korean language comprehensive technical guide covering architecture, installation, usage, and development |

## Documentation Overview

### complete-guide.md

**Language**: Korean (한국어)

**Sections**:
1. Project Overview - Purpose, features, and tech stack
2. Architecture - Flow diagrams, request processing, fsnotify mechanism
3. File Structure - Project layout and installation paths
4. Core Components - Detailed code walkthroughs
5. Installation & Configuration - Setup instructions
6. Usage - Commands, mode switching, daemon management
7. Routing Modes - Detailed mode comparison
8. Auto Mode - Failover logic, cooldown, state transitions
9. Configuration - Mode file, config.json, environment variables
10. Customization - Adding modes, changing mappings
11. Troubleshooting - Common issues and solutions
12. Development Guide - Local setup, code structure, testing

**Target Audience**:
- Developers working on rrouter
- System administrators deploying rrouter
- Users needing deep technical understanding
- Korean-speaking community

**Key Topics Covered**:
- Two-stage model transformation (rrouter → clipproxyapi)
- Auto mode bidirectional failover
- fsnotify-based zero-I/O configuration watching
- Glob pattern matching for model names
- Cooldown escalation (30min → 240min)
- Daemon management across Linux/macOS

## For AI Agents

When working with documentation:

1. **Documentation Language**:
   - `complete-guide.md` is in Korean
   - English documentation is in root `README.md`
   - Consider creating English version of complete guide

2. **Documentation Updates**:
   - Keep synchronized with code changes
   - Update version numbers and dates
   - Verify command examples are current
   - Check file paths match actual installation

3. **Key Documentation Patterns**:
   - Uses flow diagrams for architecture
   - Includes table-based comparisons
   - Provides code examples with syntax highlighting
   - Shows actual command output
   - Links configuration to code implementation

4. **Missing Documentation**:
   - English version of complete guide
   - API reference for /health endpoint
   - Configuration schema documentation
   - Migration guide from older versions
   - Performance tuning guide

5. **When to Update Docs**:
   - New features added (modes, commands, options)
   - Configuration format changes
   - Auto mode thresholds modified
   - Service file paths updated
   - Breaking changes to behavior

6. **Documentation Standards**:
   - Keep code examples testable
   - Show both systemd and launchd variants
   - Include troubleshooting for each feature
   - Provide before/after examples
   - Link to relevant source files

7. **Common Documentation Tasks**:
   - Add new mode documentation
   - Update architecture diagrams
   - Document new CLI commands
   - Add troubleshooting entries
   - Update version history
