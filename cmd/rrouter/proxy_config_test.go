package main

import (
	"encoding/json"
	"testing"
)

func TestMatchModel(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		model    string
		expected bool
	}{
		{
			name:     "wildcard matches sonnet",
			pattern:  "claude-sonnet-*",
			model:    "claude-sonnet-4-5",
			expected: true,
		},
		{
			name:     "wildcard matches opus",
			pattern:  "claude-opus-*",
			model:    "claude-opus-4-5",
			expected: true,
		},
		{
			name:     "haiku pattern does not match sonnet",
			pattern:  "claude-haiku-*",
			model:    "claude-sonnet-4-5",
			expected: false,
		},
		{
			name:     "empty pattern returns false",
			pattern:  "",
			model:    "claude-sonnet-4-5",
			expected: false,
		},
		{
			name:     "exact match",
			pattern:  "foo",
			model:    "foo",
			expected: true,
		},
		{
			name:     "star matches anything",
			pattern:  "*",
			model:    "any-model-name",
			expected: true,
		},
		{
			name:     "no match",
			pattern:  "gpt-*",
			model:    "claude-sonnet-4-5",
			expected: false,
		},
		{
			name:     "complex wildcard match",
			pattern:  "claude-*-4-5",
			model:    "claude-sonnet-4-5",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchModel(tt.pattern, tt.model)
			if result != tt.expected {
				t.Errorf("matchModel(%q, %q) = %v, want %v", tt.pattern, tt.model, result, tt.expected)
			}
		})
	}
}

func TestRewriteModelWithConfig(t *testing.T) {
	tests := []struct {
		name       string
		model      string
		modeConfig *ModeConfig
		expected   string
	}{
		{
			name:       "nil config returns original model",
			model:      "claude-sonnet-4-5",
			modeConfig: nil,
			expected:   "claude-sonnet-4-5",
		},
		{
			name:  "empty mappings returns original model",
			model: "claude-sonnet-4-5",
			modeConfig: &ModeConfig{
				Mappings: []ModelMapping{},
			},
			expected: "claude-sonnet-4-5",
		},
		{
			name:  "first matching rule wins",
			model: "claude-sonnet-4-5",
			modeConfig: &ModeConfig{
				Mappings: []ModelMapping{
					{Match: "claude-sonnet-*", Rewrite: "claude-haiku-3-5"},
					{Match: "claude-sonnet-*", Rewrite: "claude-opus-4-5"},
				},
			},
			expected: "claude-haiku-3-5",
		},
		{
			name:  "no match returns original model",
			model: "claude-sonnet-4-5",
			modeConfig: &ModeConfig{
				Mappings: []ModelMapping{
					{Match: "gpt-*", Rewrite: "claude-haiku-3-5"},
				},
			},
			expected: "claude-sonnet-4-5",
		},
		{
			name:  "exact match rewrites correctly",
			model: "claude-opus-4-5",
			modeConfig: &ModeConfig{
				Mappings: []ModelMapping{
					{Match: "claude-opus-4-5", Rewrite: "claude-haiku-3-5"},
				},
			},
			expected: "claude-haiku-3-5",
		},
		{
			name:  "wildcard star matches any model",
			model: "any-model",
			modeConfig: &ModeConfig{
				Mappings: []ModelMapping{
					{Match: "*", Rewrite: "default-model"},
				},
			},
			expected: "default-model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rewriteModelWithConfig(tt.model, tt.modeConfig)
			if result != tt.expected {
				t.Errorf("rewriteModelWithConfig(%q, %+v) = %q, want %q", tt.model, tt.modeConfig, result, tt.expected)
			}
		})
	}
}

func TestGetConfig(t *testing.T) {
	tests := []struct {
		name         string
		port         string
		upstream     string
		expectedAddr string
		expectedUp   string
	}{
		{
			name:         "default values with no env vars",
			port:         "",
			upstream:     "",
			expectedAddr: ":8316",
			expectedUp:   "http://localhost:8317",
		},
		{
			name:         "custom port",
			port:         "9999",
			upstream:     "",
			expectedAddr: ":9999",
			expectedUp:   "http://localhost:8317",
		},
		{
			name:         "port with leading colon",
			port:         ":9999",
			upstream:     "",
			expectedAddr: ":9999",
			expectedUp:   "http://localhost:8317",
		},
		{
			name:         "custom upstream",
			port:         "",
			upstream:     "http://example.com",
			expectedAddr: ":8316",
			expectedUp:   "http://example.com",
		},
		{
			name:         "both custom",
			port:         "7777",
			upstream:     "http://backend.local:8080",
			expectedAddr: ":7777",
			expectedUp:   "http://backend.local:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.port != "" {
				t.Setenv("RROUTER_PORT", tt.port)
			}
			if tt.upstream != "" {
				t.Setenv("RROUTER_UPSTREAM", tt.upstream)
			}

			addr, up := getConfig()
			if addr != tt.expectedAddr {
				t.Errorf("getConfig() addr = %q, want %q", addr, tt.expectedAddr)
			}
			if up != tt.expectedUp {
				t.Errorf("getConfig() upstream = %q, want %q", up, tt.expectedUp)
			}
		})
	}
}

func TestLoadEmbeddedConfig(t *testing.T) {
	cfg := loadEmbeddedConfig()

	if cfg == nil {
		t.Fatal("loadEmbeddedConfig() returned nil")
	}

	// Check that antigravity mode exists and has mappings
	antiGravity, ok := cfg.Modes["antigravity"]
	if !ok {
		t.Error("Expected 'antigravity' mode in embedded config")
	} else {
		if len(antiGravity.Mappings) == 0 {
			t.Error("Expected 'antigravity' mode to have mappings")
		}
	}

	// Check that claude mode exists with empty mappings (passthrough)
	claude, ok := cfg.Modes["claude"]
	if !ok {
		t.Error("Expected 'claude' mode in embedded config")
	} else {
		if len(claude.Mappings) != 0 {
			t.Errorf("Expected 'claude' mode to have empty mappings, got %d", len(claude.Mappings))
		}
	}

	// Check DefaultMode
	if cfg.DefaultMode != "claude" {
		t.Errorf("Expected DefaultMode to be 'claude', got %q", cfg.DefaultMode)
	}
}

func TestModifyRequestBody(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		modeConfig  *ModeConfig
		mode        string
		expectModel string
		expectError bool
	}{
		{
			name: "valid JSON with model field gets rewritten",
			body: `{"model": "claude-sonnet-4-5", "prompt": "hello"}`,
			modeConfig: &ModeConfig{
				Mappings: []ModelMapping{
					{Match: "claude-sonnet-*", Rewrite: "claude-haiku-3-5"},
				},
			},
			mode:        "test-mode",
			expectModel: "claude-haiku-3-5",
			expectError: false,
		},
		{
			name:        "valid JSON without model field passes through",
			body:        `{"prompt": "hello", "temperature": 0.7}`,
			modeConfig:  &ModeConfig{Mappings: []ModelMapping{}},
			mode:        "test-mode",
			expectModel: "",
			expectError: false,
		},
		{
			name:        "invalid JSON returns error",
			body:        `{invalid json`,
			modeConfig:  &ModeConfig{Mappings: []ModelMapping{}},
			mode:        "test-mode",
			expectModel: "",
			expectError: true,
		},
		{
			name:        "nil modeConfig does not rewrite",
			body:        `{"model": "claude-sonnet-4-5", "prompt": "hello"}`,
			modeConfig:  nil,
			mode:        "test-mode",
			expectModel: "claude-sonnet-4-5",
			expectError: false,
		},
		{
			name: "no matching rule leaves model unchanged",
			body: `{"model": "gpt-4", "prompt": "hello"}`,
			modeConfig: &ModeConfig{
				Mappings: []ModelMapping{
					{Match: "claude-*", Rewrite: "claude-haiku-3-5"},
				},
			},
			mode:        "test-mode",
			expectModel: "gpt-4",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := modifyRequestBody([]byte(tt.body), tt.modeConfig, tt.mode)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if tt.expectModel != "" {
				// Parse result to check model field
				var data map[string]interface{}
				if err := json.Unmarshal(result, &data); err != nil {
					t.Fatalf("Failed to parse result JSON: %v", err)
				}

				if modelVal, ok := data["model"]; ok {
					if modelStr, ok := modelVal.(string); ok {
						if modelStr != tt.expectModel {
							t.Errorf("Expected model %q, got %q", tt.expectModel, modelStr)
						}
					} else {
						t.Error("Model field is not a string")
					}
				} else {
					t.Error("Model field missing in result")
				}
			} else {
				// Verify no model field exists
				var data map[string]interface{}
				if err := json.Unmarshal(result, &data); err != nil {
					t.Fatalf("Failed to parse result JSON: %v", err)
				}
				if _, ok := data["model"]; ok {
					t.Error("Expected no model field but found one")
				}
			}
		})
	}
}
