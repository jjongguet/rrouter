package main

import (
	"log"
	"regexp"
	"strings"
)

// AgentType represents the classification of an OMC agent
type AgentType int

const (
	AgentTypeUnknown AgentType = iota // detection failed or not in lists
	AgentTypeGroup1                   // exploration/analysis agents -> readonlyModel
	AgentTypeGroup2                   // execution/write agents -> standard mapping
)

func (a AgentType) String() string {
	switch a {
		case AgentTypeGroup1:
			return "group1"
		case AgentTypeGroup2:
			return "group2"
		default:
			return "unknown"
	}
}

// AgentRoutingConfig holds agent-type-based routing configuration
type AgentRoutingConfig struct {
	Enabled      bool     `json:"enabled"`
	Group1Model  string   `json:"group1Model"`
	Group1Agents []string `json:"group1Agents"`
	Group2Agents []string `json:"group2Agents"`
}

// Package-level compiled regexes for performance (compiled once at startup)
var (
	// Matches "Agent oh-my-claudecode:{name} started" pattern (most specific)
	agentStartedPattern = regexp.MustCompile(`Agent oh-my-claudecode:(\S+)\s+started`)
	// Fallback: matches any "oh-my-claudecode:{name}" reference
	agentNamePattern = regexp.MustCompile(`oh-my-claudecode:(\S+)`)
)

// detectAgentName extracts the OMC agent name from the system prompt.
// The system prompt can be a string or an array of content blocks.
// Returns empty string if no agent name found.
func detectAgentName(data map[string]interface{}) string {
	systemVal, ok := data["system"]
	if !ok {
		return ""
	}

	var systemText string

	switch v := systemVal.(type) {
	case string:
		systemText = v
	case []interface{}:
		// system can be array of content blocks: [{"type":"text","text":"..."}]
		var parts []string
		for _, item := range v {
			if block, ok := item.(map[string]interface{}); ok {
				if text, ok := block["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		systemText = strings.Join(parts, " ")
	case nil:
		// JSON null
		return ""
	default:
		return ""
	}

	if systemText == "" {
		return ""
	}

	// Look for "Agent oh-my-claudecode:{name} started" pattern first (most specific)
	if matches := agentStartedPattern.FindStringSubmatch(systemText); len(matches) > 1 {
		return normalizeAgentName(matches[1])
	}

	// Fallback: any oh-my-claudecode:{name} reference
	if matches := agentNamePattern.FindStringSubmatch(systemText); len(matches) > 1 {
		return normalizeAgentName(matches[1])
	}

	return ""
}

// normalizeAgentName strips trailing punctuation and converts to lowercase
func normalizeAgentName(name string) string {
	// Remove trailing punctuation that might be captured by \S+
	name = strings.TrimRight(name, ".,;:!?\"')`")
	return strings.ToLower(name)
}

// classifyAgent determines if an agent is group1 or group2 based on config
func classifyAgent(agentName string, routingConfig *AgentRoutingConfig) AgentType {
	if routingConfig == nil || !routingConfig.Enabled || agentName == "" {
		return AgentTypeUnknown
	}

	for _, agent := range routingConfig.Group1Agents {
		if strings.EqualFold(agent, agentName) {
			return AgentTypeGroup1
		}
	}

	for _, agent := range routingConfig.Group2Agents {
		if strings.EqualFold(agent, agentName) {
			return AgentTypeGroup2
		}
	}

	// Agent name detected but not in either list -> unknown (safe fallback)
	return AgentTypeUnknown
}

// validateAgentRoutingConfig checks the config for common errors
func validateAgentRoutingConfig(cfg *AgentRoutingConfig, modeName string) {
	if cfg == nil || !cfg.Enabled {
		return
	}

	// Warn if enabled but group1Model is empty
	if cfg.Group1Model == "" {
		log.Printf("[WARN] Mode '%s' has agentRouting.enabled=true but group1Model is empty. Group1 agents will use empty model string.", modeName)
	}

	// Check for duplicates between lists
	group1Set := make(map[string]bool)
	for _, a := range cfg.Group1Agents {
		group1Set[strings.ToLower(a)] = true
	}
	for _, a := range cfg.Group2Agents {
		if group1Set[strings.ToLower(a)] {
			log.Printf("[WARN] Mode '%s': Agent '%s' is in both group1Agents and group2Agents. group1 will take precedence.", modeName, a)
		}
	}
}
