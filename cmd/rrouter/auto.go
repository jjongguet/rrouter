package main

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

const (
	failureThreshold = 3 // consecutive 429/5xx to trigger switch
	timeoutThreshold = 2 // consecutive timeouts to trigger switch
	initialCooldown  = 30 * time.Minute
	maxCooldown      = 4 * time.Hour
)

// autoState tracks in-memory routing state for "auto" mode.
// Supports BIDIRECTIONAL failover: either target can fail and switch to the other.
// This state is intentionally NOT persisted to disk.
// Proxy restart = fresh start on defaultTarget.
type autoState struct {
	mu sync.Mutex

	defaultTarget string // starting target (e.g., "antigravity")
	currentTarget string // current active target
	previousTarget string // what we switched FROM (for logging/health)

	failureCount int
	timeoutCount int
	switched     bool // true if we've switched away from defaultTarget
	switchedAt   time.Time

	cooldownDuration time.Duration
	cooldownTimer    *time.Timer
	generation       uint64

	switchCount  atomic.Int64
	healthySince time.Time
}

func newAutoState(defaultTarget string) *autoState {
	if defaultTarget == "" {
		defaultTarget = "antigravity"
	}
	return &autoState{
		defaultTarget:    defaultTarget,
		currentTarget:    defaultTarget,
		cooldownDuration: initialCooldown,
	}
}

// newAutoStateForTest creates an autoState with custom initial cooldown for testing.
func newAutoStateForTest(defaultTarget string, cooldown time.Duration) *autoState {
	if defaultTarget == "" {
		defaultTarget = "antigravity"
	}
	return &autoState{
		defaultTarget:    defaultTarget,
		currentTarget:    defaultTarget,
		cooldownDuration: cooldown,
	}
}

// oppositeTarget returns the other target.
func oppositeTarget(target string) string {
	if target == "antigravity" {
		return "claude"
	}
	return "antigravity"
}

// resolveRouting maps user's mode intent to a concrete routing target.
// For "auto" mode, returns currentTarget based on in-memory state.
// For explicit modes, returns the mode as-is.
func (s *autoState) resolveRouting(intent string) string {
	if intent != "auto" {
		return intent
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.currentTarget
}

// recordUpstreamResponse updates auto-switch state based on upstream response.
// Only has effect when called (caller gates on intent == "auto").
//
// statusCode: the HTTP status from upstream (0 for timeout/connection error)
// isTimeout: true if the request timed out or connection was refused
func (s *autoState) recordUpstreamResponse(statusCode int, isTimeout bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Success: only 2xx resets counters (4xx/5xx are failures that trigger switch)
	if !isTimeout && statusCode >= 200 && statusCode < 300 {
		if s.failureCount > 0 || s.timeoutCount > 0 {
			log.Printf("[AUTO] Success (HTTP %d) on %s -- resetting failure counters (was: %d failures, %d timeouts)",
				statusCode, s.currentTarget, s.failureCount, s.timeoutCount)
		}
		s.failureCount = 0
		s.timeoutCount = 0

		// Track healthy period for cooldown decay
		if s.healthySince.IsZero() {
			s.healthySince = time.Now()
		}
		// Cooldown decay: if healthy for 2x current cooldown, reset to initial
		if s.cooldownDuration > initialCooldown && !s.healthySince.IsZero() {
			healthyDuration := time.Since(s.healthySince)
			if healthyDuration >= s.cooldownDuration*2 {
				log.Printf("[AUTO] Sustained healthy operation (%s) -- resetting cooldown from %s to %s",
					healthyDuration.Round(time.Second), s.cooldownDuration, initialCooldown)
				s.cooldownDuration = initialCooldown
			}
		}
		return
	}

	// Any failure resets the healthy streak
	s.healthySince = time.Time{}

	// Timeout handling (tracked separately from HTTP errors)
	if isTimeout {
		s.timeoutCount++
		s.failureCount = 0 // timeouts and HTTP failures tracked separately
		log.Printf("[AUTO] Timeout on %s (consecutive: %d/%d)", s.currentTarget, s.timeoutCount, timeoutThreshold)
		if s.timeoutCount >= timeoutThreshold {
			s.triggerSwitch("timeout")
		}
		return
	}

	// 4xx or 5xx (any non-2xx error triggers failover counting)
	if statusCode >= 400 {
		s.failureCount++
		s.timeoutCount = 0 // timeouts and HTTP failures tracked separately
		log.Printf("[AUTO] Upstream error HTTP %d on %s (consecutive: %d/%d)", statusCode, s.currentTarget, s.failureCount, failureThreshold)
		if s.failureCount >= failureThreshold {
			s.triggerSwitch(fmt.Sprintf("HTTP %d", statusCode))
		}
		return
	}
}

// triggerSwitch flips the current target to the opposite one.
// MUST be called with s.mu held.
func (s *autoState) triggerSwitch(reason string) {
	from := s.currentTarget
	to := oppositeTarget(from)

	s.previousTarget = from
	s.currentTarget = to
	s.switched = (s.currentTarget != s.defaultTarget)
	s.switchedAt = time.Now()
	s.switchCount.Add(1)
	s.failureCount = 0
	s.timeoutCount = 0
	s.healthySince = time.Time{}

	// Escalate cooldown on repeated switches (not the first one)
	if s.switchCount.Load() > 1 {
		s.cooldownDuration = min(s.cooldownDuration*2, maxCooldown)
	}

	log.Printf("=====================================================")
	log.Printf("[AUTO] SWITCHING: %s -> %s", from, to)
	log.Printf("[AUTO] Reason: %s (threshold reached)", reason)
	log.Printf("[AUTO] Cooldown: %s (will try %s again after)", s.cooldownDuration, from)
	log.Printf("=====================================================")

	// Increment generation BEFORE starting cooldown
	s.generation++
	s.startCooldown(from)
}

// startCooldown starts the recovery timer. When it fires, routing
// switches back to the previous target to test if it has recovered.
// MUST be called with s.mu held.
func (s *autoState) startCooldown(retryTarget string) {
	if s.cooldownTimer != nil {
		s.cooldownTimer.Stop()
	}

	duration := s.cooldownDuration
	gen := s.generation

	s.cooldownTimer = time.AfterFunc(duration, func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		// Only act if generation matches (prevents stale timer race condition)
		if s.generation != gen {
			log.Printf("[AUTO] Stale cooldown timer fired (gen %d, current %d) -- ignoring", gen, s.generation)
			return
		}

		// Switch back to retry the previous target
		from := s.currentTarget
		s.currentTarget = retryTarget
		s.switched = (s.currentTarget != s.defaultTarget)
		s.failureCount = 0
		s.timeoutCount = 0
		s.healthySince = time.Now()

		log.Printf("=====================================================")
		log.Printf("[AUTO] COOLDOWN EXPIRED: %s -> %s (retrying)", from, retryTarget)
		log.Printf("[AUTO] Next cooldown if %s fails again: %s", retryTarget, min(s.cooldownDuration*2, maxCooldown))
		log.Printf("=====================================================")
	})
}

// reset clears all auto-switch state. Called when user manually
// switches to an explicit mode.
func (s *autoState) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.failureCount = 0
	s.timeoutCount = 0
	s.currentTarget = s.defaultTarget
	s.previousTarget = ""
	s.switched = false
	s.switchedAt = time.Time{}
	s.cooldownDuration = initialCooldown
	s.healthySince = time.Time{}
	s.switchCount.Store(0)
	s.generation++ // invalidate any pending cooldown timer callbacks
	if s.cooldownTimer != nil {
		s.cooldownTimer.Stop()
		s.cooldownTimer = nil
	}

	log.Printf("[AUTO] State reset (manual mode switch)")
}

// HealthInfo returns auto-switch state for the /health endpoint.
func (s *autoState) HealthInfo() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	info := map[string]interface{}{
		"defaultTarget":   s.defaultTarget,
		"currentTarget":   s.currentTarget,
		"autoSwitched":    s.switched,
		"autoSwitchCount": s.switchCount.Load(),
		"failureCount":    s.failureCount,
		"timeoutCount":    s.timeoutCount,
	}

	if s.previousTarget != "" {
		info["previousTarget"] = s.previousTarget
	}

	if s.switched {
		info["switchedAt"] = s.switchedAt.Format(time.RFC3339)
		remaining := time.Until(s.switchedAt.Add(s.cooldownDuration))
		if remaining > 0 {
			info["cooldownRemaining"] = remaining.Round(time.Second).String()
		} else {
			info["cooldownRemaining"] = "expiring soon"
		}
		info["cooldownDuration"] = s.cooldownDuration.String()
	}

	return info
}
