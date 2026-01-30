package main

import (
	"testing"
	"time"
)

// ========== 1. resolveRouting tests ==========

func TestResolveRouting_NonAutoIntent(t *testing.T) {
	s := newAutoState("antigravity")

	tests := []struct {
		name   string
		intent string
		want   string
	}{
		{
			name:   "explicit claude",
			intent: "claude",
			want:   "claude",
		},
		{
			name:   "explicit antigravity",
			intent: "antigravity",
			want:   "antigravity",
		},
		{
			name:   "other explicit mode",
			intent: "someothermode",
			want:   "someothermode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.resolveRouting(tt.intent)
			if got != tt.want {
				t.Errorf("resolveRouting(%q) = %q, want %q", tt.intent, got, tt.want)
			}
		})
	}
}

func TestResolveRouting_AutoNotSwitched(t *testing.T) {
	s := newAutoState("antigravity")
	got := s.resolveRouting("auto")
	want := "antigravity"
	if got != want {
		t.Errorf("resolveRouting(auto) when not switched = %q, want %q", got, want)
	}
}

func TestResolveRouting_AutoSwitched(t *testing.T) {
	s := newAutoState("antigravity")
	s.mu.Lock()
	s.switched = true
	s.currentTarget = "claude" // new bidirectional model uses currentTarget
	s.mu.Unlock()

	got := s.resolveRouting("auto")
	want := "claude"
	if got != want {
		t.Errorf("resolveRouting(auto) when switched = %q, want %q", got, want)
	}
}

// ========== 2. recordUpstreamResponse - success resets ==========

func TestRecordUpstreamResponse_SuccessResets(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{name: "HTTP 200", statusCode: 200},
		{name: "HTTP 201", statusCode: 201},
		{name: "HTTP 202", statusCode: 202},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newAutoState("antigravity")
			s.mu.Lock()
			s.failureCount = 2
			s.timeoutCount = 1
			s.mu.Unlock()

			s.recordUpstreamResponse(tt.statusCode, false)

			s.mu.Lock()
			defer s.mu.Unlock()
			if s.failureCount != 0 {
				t.Errorf("failureCount = %d, want 0", s.failureCount)
			}
			if s.timeoutCount != 0 {
				t.Errorf("timeoutCount = %d, want 0", s.timeoutCount)
			}
			if s.switched {
				t.Error("switched = true, want false (no switch on success)")
			}
		})
	}
}

func TestRecordUpstreamResponse_SuccessAfterFailures_NoSwitch(t *testing.T) {
	s := newAutoState("antigravity")
	s.recordUpstreamResponse(500, false) // failure 1
	s.recordUpstreamResponse(500, false) // failure 2

	s.mu.Lock()
	if s.failureCount != 2 {
		t.Fatalf("setup: failureCount = %d, want 2", s.failureCount)
	}
	s.mu.Unlock()

	s.recordUpstreamResponse(200, false) // success resets

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failureCount != 0 {
		t.Errorf("failureCount = %d, want 0 after success", s.failureCount)
	}
	if s.switched {
		t.Error("switched = true, want false (only 2 failures)")
	}
}

// ========== 3. recordUpstreamResponse - failure counting ==========

func TestRecordUpstreamResponse_ThreeConsecutive400s(t *testing.T) {
	s := newAutoState("antigravity")
	s.recordUpstreamResponse(400, false)
	s.recordUpstreamResponse(400, false)
	s.recordUpstreamResponse(400, false)

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.switched {
		t.Error("switched = false, want true after 3 consecutive 400s")
	}
	if s.currentTarget != "claude" {
		t.Errorf("currentTarget = %s, want claude", s.currentTarget)
	}
}

func TestRecordUpstreamResponse_ThreeConsecutive500s(t *testing.T) {
	s := newAutoState("antigravity")
	s.recordUpstreamResponse(500, false)
	s.recordUpstreamResponse(500, false)

	s.mu.Lock()
	if s.switched {
		t.Fatalf("switched after 2 failures, want false")
	}
	s.mu.Unlock()

	s.recordUpstreamResponse(500, false) // third failure triggers

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.switched {
		t.Error("switched = false after 3 failures, want true")
	}
	if s.failureCount != 0 {
		t.Errorf("failureCount = %d after switch, want 0 (reset)", s.failureCount)
	}
}

func TestRecordUpstreamResponse_TwoConsecutive429s_NoSwitch(t *testing.T) {
	s := newAutoState("antigravity")
	s.recordUpstreamResponse(429, false)
	s.recordUpstreamResponse(429, false)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.switched {
		t.Error("switched = true after 2x 429, want false (need 3)")
	}
	if s.failureCount != 2 {
		t.Errorf("failureCount = %d, want 2", s.failureCount)
	}
}

func TestRecordUpstreamResponse_ThreeConsecutive429s(t *testing.T) {
	s := newAutoState("antigravity")
	s.recordUpstreamResponse(429, false)
	s.recordUpstreamResponse(429, false)
	s.recordUpstreamResponse(429, false)

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.switched {
		t.Error("switched = false after 3x 429, want true")
	}
}

func TestRecordUpstreamResponse_Mixed500And429Accumulate(t *testing.T) {
	s := newAutoState("antigravity")
	s.recordUpstreamResponse(500, false) // failure 1
	s.recordUpstreamResponse(429, false) // failure 2
	s.recordUpstreamResponse(503, false) // failure 3

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.switched {
		t.Error("switched = false after mixed 500+429, want true")
	}
}

// ========== 4. recordUpstreamResponse - timeout counting ==========

func TestRecordUpstreamResponse_TwoConsecutiveTimeouts(t *testing.T) {
	s := newAutoState("antigravity")
	s.recordUpstreamResponse(0, true) // timeout 1

	s.mu.Lock()
	if s.switched {
		t.Fatalf("switched after 1 timeout, want false")
	}
	s.mu.Unlock()

	s.recordUpstreamResponse(0, true) // timeout 2 triggers

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.switched {
		t.Error("switched = false after 2 timeouts, want true")
	}
	if s.timeoutCount != 0 {
		t.Errorf("timeoutCount = %d after switch, want 0 (reset)", s.timeoutCount)
	}
}

func TestRecordUpstreamResponse_OneTimeout_NoSwitch(t *testing.T) {
	s := newAutoState("antigravity")
	s.recordUpstreamResponse(0, true)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.switched {
		t.Error("switched = true after 1 timeout, want false")
	}
	if s.timeoutCount != 1 {
		t.Errorf("timeoutCount = %d, want 1", s.timeoutCount)
	}
}

func TestRecordUpstreamResponse_TimeoutResetsFailureCount(t *testing.T) {
	s := newAutoState("antigravity")
	s.recordUpstreamResponse(500, false) // failureCount = 1
	s.recordUpstreamResponse(500, false) // failureCount = 2

	s.mu.Lock()
	if s.failureCount != 2 {
		t.Fatalf("setup: failureCount = %d, want 2", s.failureCount)
	}
	s.mu.Unlock()

	s.recordUpstreamResponse(0, true) // timeout resets failureCount

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failureCount != 0 {
		t.Errorf("failureCount = %d after timeout, want 0 (separate tracking)", s.failureCount)
	}
	if s.timeoutCount != 1 {
		t.Errorf("timeoutCount = %d, want 1", s.timeoutCount)
	}
}

func TestRecordUpstreamResponse_HTTPFailureResetsTimeoutCount(t *testing.T) {
	s := newAutoState("antigravity")
	s.recordUpstreamResponse(0, true) // timeoutCount = 1

	s.mu.Lock()
	if s.timeoutCount != 1 {
		t.Fatalf("setup: timeoutCount = %d, want 1", s.timeoutCount)
	}
	s.mu.Unlock()

	s.recordUpstreamResponse(500, false) // HTTP error resets timeoutCount

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timeoutCount != 0 {
		t.Errorf("timeoutCount = %d after HTTP error, want 0 (separate tracking)", s.timeoutCount)
	}
	if s.failureCount != 1 {
		t.Errorf("failureCount = %d, want 1", s.failureCount)
	}
}

// ========== 5. triggerSwitch behavior ==========

func TestTriggerSwitch_StateChanges(t *testing.T) {
	s := newAutoState("antigravity")
	s.mu.Lock()
	s.failureCount = 3
	s.timeoutCount = 1
	beforeGen := s.generation
	s.triggerSwitch("test")
	s.mu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.switched {
		t.Error("switched = false after triggerSwitch, want true")
	}
	if s.switchCount.Load() != 1 {
		t.Errorf("switchCount = %d, want 1", s.switchCount.Load())
	}
	if s.failureCount != 0 {
		t.Errorf("failureCount = %d, want 0 (reset after switch)", s.failureCount)
	}
	if s.timeoutCount != 0 {
		t.Errorf("timeoutCount = %d, want 0 (reset after switch)", s.timeoutCount)
	}
	if !s.switchedAt.After(time.Now().Add(-time.Second)) {
		t.Error("switchedAt not set recently")
	}
	if s.generation != beforeGen+1 {
		t.Errorf("generation = %d, want %d (incremented)", s.generation, beforeGen+1)
	}
}

// ========== 6. Cooldown escalation ==========

func TestCooldownEscalation_FirstSwitch(t *testing.T) {
	s := newAutoState("antigravity")
	s.mu.Lock()
	s.triggerSwitch("first")
	s.mu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cooldownDuration != initialCooldown {
		t.Errorf("cooldownDuration = %s after first switch, want %s (no escalation yet)",
			s.cooldownDuration, initialCooldown)
	}
}

func TestCooldownEscalation_SecondSwitch(t *testing.T) {
	s := newAutoState("antigravity")
	s.mu.Lock()
	s.triggerSwitch("first")
	s.triggerSwitch("second")
	s.mu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	want := initialCooldown * 2
	if s.cooldownDuration != want {
		t.Errorf("cooldownDuration = %s after second switch, want %s",
			s.cooldownDuration, want)
	}
}

func TestCooldownEscalation_ThirdSwitch(t *testing.T) {
	s := newAutoState("antigravity")
	s.mu.Lock()
	s.triggerSwitch("first")
	s.triggerSwitch("second")
	s.triggerSwitch("third")
	s.mu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	want := initialCooldown * 4 // 30m * 2 * 2
	if s.cooldownDuration != want {
		t.Errorf("cooldownDuration = %s after third switch, want %s",
			s.cooldownDuration, want)
	}
}

func TestCooldownEscalation_CappedAtMax(t *testing.T) {
	s := newAutoState("antigravity")
	s.mu.Lock()
	// Manually escalate to near max
	s.cooldownDuration = maxCooldown / 2
	s.switchCount.Store(1) // already had one switch
	s.triggerSwitch("first")
	s.triggerSwitch("second")
	s.mu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cooldownDuration > maxCooldown {
		t.Errorf("cooldownDuration = %s, exceeds maxCooldown %s",
			s.cooldownDuration, maxCooldown)
	}
	if s.cooldownDuration != maxCooldown {
		t.Errorf("cooldownDuration = %s, want %s (capped)",
			s.cooldownDuration, maxCooldown)
	}
}

// ========== 7. Cooldown timer fires ==========

func TestCooldownTimer_Fires_SwitchesBack(t *testing.T) {
	s := newAutoStateForTest("antigravity", 50 * time.Millisecond)
	s.recordUpstreamResponse(500, false)
	s.recordUpstreamResponse(500, false)
	s.recordUpstreamResponse(500, false) // triggers switch

	s.mu.Lock()
	if !s.switched {
		t.Fatalf("setup: not switched after 3 failures")
	}
	s.mu.Unlock()

	time.Sleep(100 * time.Millisecond) // wait for cooldown

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.switched {
		t.Error("switched = true after cooldown expired, want false (switched back)")
	}
	if s.failureCount != 0 {
		t.Errorf("failureCount = %d after cooldown, want 0", s.failureCount)
	}
	if s.timeoutCount != 0 {
		t.Errorf("timeoutCount = %d after cooldown, want 0", s.timeoutCount)
	}
}

// ========== 8. Generation counter (timer race prevention) ==========

func TestGeneration_ResetInvalidatesTimer(t *testing.T) {
	s := newAutoStateForTest("antigravity", 100 * time.Millisecond)
	s.recordUpstreamResponse(500, false)
	s.recordUpstreamResponse(500, false)
	s.recordUpstreamResponse(500, false) // triggers switch

	s.mu.Lock()
	if !s.switched {
		t.Fatalf("setup: not switched after 3 failures")
	}
	genAtSwitch := s.generation
	s.mu.Unlock()

	time.Sleep(10 * time.Millisecond) // let timer start
	s.reset()                         // reset before timer fires

	s.mu.Lock()
	if s.generation == genAtSwitch {
		t.Error("generation unchanged after reset, want incremented")
	}
	s.mu.Unlock()

	time.Sleep(150 * time.Millisecond) // wait past original cooldown

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.switched {
		t.Error("switched = true, want false (reset should prevent timer from switching back)")
	}
	// State should remain as reset set it
	if s.switchCount.Load() != 0 {
		t.Errorf("switchCount = %d, want 0 (reset)", s.switchCount.Load())
	}
}

func TestGeneration_IncrementsOnTriggerSwitch(t *testing.T) {
	s := newAutoState("antigravity")
	s.mu.Lock()
	gen1 := s.generation
	s.triggerSwitch("test")
	gen2 := s.generation
	s.mu.Unlock()

	if gen2 != gen1+1 {
		t.Errorf("generation = %d after triggerSwitch, want %d", gen2, gen1+1)
	}
}

// ========== 9. reset() ==========

func TestReset_ClearsAllState(t *testing.T) {
	s := newAutoStateForTest("antigravity", 50 * time.Millisecond)
	s.recordUpstreamResponse(500, false)
	s.recordUpstreamResponse(500, false)
	s.recordUpstreamResponse(500, false) // triggers switch

	s.mu.Lock()
	if !s.switched {
		t.Fatalf("setup: not switched")
	}
	if s.switchCount.Load() != 1 {
		t.Fatalf("setup: switchCount = %d, want 1", s.switchCount.Load())
	}
	beforeGen := s.generation
	s.mu.Unlock()

	s.reset()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failureCount != 0 {
		t.Errorf("failureCount = %d, want 0", s.failureCount)
	}
	if s.timeoutCount != 0 {
		t.Errorf("timeoutCount = %d, want 0", s.timeoutCount)
	}
	if s.switched {
		t.Error("switched = true, want false")
	}
	if !s.switchedAt.IsZero() {
		t.Error("switchedAt not zeroed")
	}
	if s.cooldownDuration != initialCooldown {
		t.Errorf("cooldownDuration = %s, want %s", s.cooldownDuration, initialCooldown)
	}
	if s.switchCount.Load() != 0 {
		t.Errorf("switchCount = %d, want 0", s.switchCount.Load())
	}
	if s.generation != beforeGen+1 {
		t.Errorf("generation = %d, want %d (incremented)", s.generation, beforeGen+1)
	}
}

// ========== 10. HealthInfo() ==========

func TestHealthInfo_NotSwitched(t *testing.T) {
	s := newAutoState("antigravity")
	s.mu.Lock()
	s.failureCount = 1
	s.timeoutCount = 2
	s.mu.Unlock()

	info := s.HealthInfo()

	if info["autoSwitched"] != false {
		t.Errorf("autoSwitched = %v, want false", info["autoSwitched"])
	}
	if info["autoSwitchCount"] != int64(0) {
		t.Errorf("autoSwitchCount = %v, want 0", info["autoSwitchCount"])
	}
	if info["failureCount"] != 1 {
		t.Errorf("failureCount = %v, want 1", info["failureCount"])
	}
	if info["timeoutCount"] != 2 {
		t.Errorf("timeoutCount = %v, want 2", info["timeoutCount"])
	}
	if _, exists := info["switchedAt"]; exists {
		t.Error("switchedAt present when not switched, want absent")
	}
}

func TestHealthInfo_Switched(t *testing.T) {
	s := newAutoStateForTest("antigravity", 1 * time.Hour)
	s.recordUpstreamResponse(500, false)
	s.recordUpstreamResponse(500, false)
	s.recordUpstreamResponse(500, false) // triggers switch

	info := s.HealthInfo()

	if info["autoSwitched"] != true {
		t.Errorf("autoSwitched = %v, want true", info["autoSwitched"])
	}
	if info["autoSwitchCount"] != int64(1) {
		t.Errorf("autoSwitchCount = %v, want 1", info["autoSwitchCount"])
	}
	if _, exists := info["switchedAt"]; !exists {
		t.Error("switchedAt missing when switched")
	}
	if _, exists := info["cooldownRemaining"]; !exists {
		t.Error("cooldownRemaining missing when switched")
	}
	if info["cooldownDuration"] != "1h0m0s" {
		t.Errorf("cooldownDuration = %v, want 1h0m0s", info["cooldownDuration"])
	}
}

// ========== 11. Cooldown decay ==========

func TestCooldownDecay_AfterSustainedHealth(t *testing.T) {
	// Use production values for this test since decay check uses global initialCooldown constant
	s := newAutoState("antigravity") // starts with 30m cooldown
	s.mu.Lock()
	// Simulate escalated cooldown from prior switches
	s.cooldownDuration = 1 * time.Hour
	s.switchCount.Store(2)
	s.mu.Unlock()

	// Record initial success to start healthy streak
	s.recordUpstreamResponse(200, false)

	s.mu.Lock()
	if s.healthySince.IsZero() {
		t.Fatal("healthySince not set after success")
	}
	// Manually adjust healthySince to simulate sustained health
	// Need > 2x current cooldown (1h), so use 2h + 1s
	s.healthySince = time.Now().Add(-(2*time.Hour + time.Second))
	s.mu.Unlock()

	// Another success should trigger decay
	s.recordUpstreamResponse(200, false)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cooldownDuration != initialCooldown {
		t.Errorf("cooldownDuration = %s after sustained health, want %s (reset to initial)",
			s.cooldownDuration, initialCooldown)
	}
}

func TestCooldownDecay_NotTriggeredTooEarly(t *testing.T) {
	// Use production values for this test since decay check uses global initialCooldown constant
	s := newAutoState("antigravity")
	s.mu.Lock()
	s.cooldownDuration = 1 * time.Hour
	s.switchCount.Store(2)
	s.mu.Unlock()

	s.recordUpstreamResponse(200, false)

	s.mu.Lock()
	if s.healthySince.IsZero() {
		t.Fatal("healthySince not set")
	}
	// Only 1h ago (< 2x 1h = 2h)
	s.healthySince = time.Now().Add(-1 * time.Hour)
	s.mu.Unlock()

	s.recordUpstreamResponse(200, false)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cooldownDuration != 1*time.Hour {
		t.Errorf("cooldownDuration = %s, want 1h (no decay yet)", s.cooldownDuration)
	}
}

// ========== 12. Bidirectional switching ==========

func TestBidirectionalSwitch_FailuresCauseSwitchBack(t *testing.T) {
	s := newAutoState("antigravity")
	s.recordUpstreamResponse(500, false)
	s.recordUpstreamResponse(500, false)
	s.recordUpstreamResponse(500, false) // triggers switch antigravity -> claude

	s.mu.Lock()
	if !s.switched {
		t.Fatalf("setup: not switched after 3 failures")
	}
	if s.switchCount.Load() != 1 {
		t.Fatalf("setup: switchCount = %d, want 1", s.switchCount.Load())
	}
	if s.currentTarget != "claude" {
		t.Fatalf("setup: currentTarget = %s, want claude", s.currentTarget)
	}
	s.mu.Unlock()

	// More failures while on claude - should switch BACK to antigravity (bidirectional)
	s.recordUpstreamResponse(500, false)
	s.recordUpstreamResponse(500, false)
	s.recordUpstreamResponse(500, false)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.switchCount.Load() != 2 {
		t.Errorf("switchCount = %d after bidirectional failures, want 2", s.switchCount.Load())
	}
	if s.currentTarget != "antigravity" {
		t.Errorf("currentTarget = %s after bidirectional switch, want antigravity", s.currentTarget)
	}
}

// ========== Additional edge cases ==========

func TestHealthySince_ResetOnFailure(t *testing.T) {
	s := newAutoState("antigravity")
	s.recordUpstreamResponse(200, false) // starts healthy streak

	s.mu.Lock()
	if s.healthySince.IsZero() {
		t.Fatal("healthySince not set after success")
	}
	s.mu.Unlock()

	s.recordUpstreamResponse(500, false) // failure resets

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.healthySince.IsZero() {
		t.Error("healthySince not reset after failure")
	}
}

func TestHealthySince_ResetOnTimeout(t *testing.T) {
	s := newAutoState("antigravity")
	s.recordUpstreamResponse(200, false)

	s.mu.Lock()
	if s.healthySince.IsZero() {
		t.Fatal("healthySince not set after success")
	}
	s.mu.Unlock()

	s.recordUpstreamResponse(0, true) // timeout resets

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.healthySince.IsZero() {
		t.Error("healthySince not reset after timeout")
	}
}

func TestTriggerSwitch_SetsHealthySinceToZero(t *testing.T) {
	s := newAutoState("antigravity")
	s.recordUpstreamResponse(200, false)

	s.mu.Lock()
	if s.healthySince.IsZero() {
		t.Fatal("healthySince not set after success")
	}
	s.triggerSwitch("test")
	s.mu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.healthySince.IsZero() {
		t.Error("healthySince not reset after triggerSwitch")
	}
}

func TestCooldownTimer_SetsHealthySince(t *testing.T) {
	s := newAutoStateForTest("antigravity", 50 * time.Millisecond)
	s.recordUpstreamResponse(500, false)
	s.recordUpstreamResponse(500, false)
	s.recordUpstreamResponse(500, false)

	time.Sleep(100 * time.Millisecond) // wait for cooldown to expire

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.healthySince.IsZero() {
		t.Error("healthySince not set after cooldown expired")
	}
}
