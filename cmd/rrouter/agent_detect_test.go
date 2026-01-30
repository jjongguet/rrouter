package main

import (
	"testing"
)

func TestDetectAgentName(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]interface{}
		expected string
	}{
		{
			name: "agent started pattern - basic",
			data: map[string]interface{}{
				"system": "Agent oh-my-claudecode:explore started",
			},
			expected: "explore",
		},
		{
			name: "agent started pattern - with suffix",
			data: map[string]interface{}{
				"system": "Agent oh-my-claudecode:architect-medium started (abc123)",
			},
			expected: "architect-medium",
		},
		{
			name: "fallback pattern - no started",
			data: map[string]interface{}{
				"system": "Using oh-my-claudecode:critic for review",
			},
			expected: "critic",
		},
		{
			name: "system as array of content blocks",
			data: map[string]interface{}{
				"system": []interface{}{
					map[string]interface{}{"type": "text", "text": "First part"},
					map[string]interface{}{"type": "text", "text": "Agent oh-my-claudecode:researcher started"},
				},
			},
			expected: "researcher",
		},
		{
			name: "no system field",
			data: map[string]interface{}{
				"model": "claude-opus-4",
			},
			expected: "",
		},
		{
			name: "empty system string",
			data: map[string]interface{}{
				"system": "",
			},
			expected: "",
		},
		{
			name: "null system field",
			data: map[string]interface{}{
				"system": nil,
			},
			expected: "",
		},
		{
			name: "system without agent pattern",
			data: map[string]interface{}{
				"system": "You are a helpful assistant",
			},
			expected: "",
		},
		{
			name: "agent name with trailing punctuation",
			data: map[string]interface{}{
				"system": "Using oh-my-claudecode:analyst.",
			},
			expected: "analyst",
		},
		{
			name: "multiple agent references - started wins",
			data: map[string]interface{}{
				"system": "Consider oh-my-claudecode:executor for code. Agent oh-my-claudecode:explore started",
			},
			expected: "explore",
		},
		{
			name: "case preserved then lowercased",
			data: map[string]interface{}{
				"system": "Agent oh-my-claudecode:Architect-LOW started",
			},
			expected: "architect-low",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectAgentName(tt.data)
			if result != tt.expected {
				t.Errorf("detectAgentName() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestClassifyAgent(t *testing.T) {
	config := &AgentRoutingConfig{
		Enabled:     true,
		Group1Model: "gemini-3-pro-preview",
		Group1Agents: []string{
			"explore", "explore-medium", "explore-high",
			"architect", "architect-medium", "architect-low",
			"researcher", "researcher-low",
			"critic", "analyst",
		},
		Group2Agents: []string{
			"executor", "executor-high", "executor-low",
			"designer", "writer", "planner",
		},
	}

	tests := []struct {
		name      string
		agentName string
		config    *AgentRoutingConfig
		expected  AgentType
	}{
		{"group1 agent - explore", "explore", config, AgentTypeGroup1},
		{"group1 agent - architect-medium", "architect-medium", config, AgentTypeGroup1},
		{"group1 agent - critic", "critic", config, AgentTypeGroup1},
		{"group2 agent - executor", "executor", config, AgentTypeGroup2},
		{"group2 agent - designer", "designer", config, AgentTypeGroup2},
		{"unknown agent - not in lists", "unknown-agent", config, AgentTypeUnknown},
		{"empty agent name", "", config, AgentTypeUnknown},
		{"nil config", "explore", nil, AgentTypeUnknown},
		{"disabled config", "explore", &AgentRoutingConfig{Enabled: false}, AgentTypeUnknown},
		{"case insensitive - EXPLORE", "EXPLORE", config, AgentTypeGroup1},
		{"case insensitive - Architect-Low", "Architect-Low", config, AgentTypeGroup1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyAgent(tt.agentName, tt.config)
			if result != tt.expected {
				t.Errorf("classifyAgent(%q) = %v, want %v", tt.agentName, result, tt.expected)
			}
		})
	}
}

func TestAgentTypeString(t *testing.T) {
	tests := []struct {
		agentType AgentType
		expected  string
	}{
		{AgentTypeUnknown, "unknown"},
		{AgentTypeGroup1, "group1"},
		{AgentTypeGroup2, "group2"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.agentType.String(); got != tt.expected {
				t.Errorf("AgentType.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestNormalizeAgentName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"explore", "explore"},
		{"EXPLORE", "explore"},
		{"architect-medium", "architect-medium"},
		{"agent.", "agent"},
		{"agent,", "agent"},
		{"agent)", "agent"},
		{"agent`", "agent"},
		{"Agent-LOW", "agent-low"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := normalizeAgentName(tt.input); got != tt.expected {
				t.Errorf("normalizeAgentName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
