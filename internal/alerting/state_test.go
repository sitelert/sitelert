package alerting

import (
	"sync"
	"testing"
	"time"
)

func TestStateManager_Get_NewService(t *testing.T) {
	sm := NewStateManager()

	state := sm.Get("svc-1")

	if state == nil {
		t.Fatal("Get returned nil")
	}
	if state.State != StateUnknown {
		t.Errorf("initial state = %q, want %q", state.State, StateUnknown)
	}
	if state.ConsecutiveFailures != 0 {
		t.Errorf("initial failures = %d, want 0", state.ConsecutiveFailures)
	}
	if state.DownNotified {
		t.Error("initial DownNotified should be false")
	}
}

func TestStateManager_Get_ExistingService(t *testing.T) {
	sm := NewStateManager()

	// First get creates the state
	state1 := sm.Get("svc-1")
	state1.State = StateUp
	state1.ConsecutiveFailures = 5

	// Second get returns same state
	state2 := sm.Get("svc-1")

	if state2.State != StateUp {
		t.Errorf("state = %q, want %q", state2.State, StateUp)
	}
	if state2.ConsecutiveFailures != 5 {
		t.Errorf("failures = %d, want 5", state2.ConsecutiveFailures)
	}
}

func TestStateManager_Get_DifferentServices(t *testing.T) {
	sm := NewStateManager()

	state1 := sm.Get("svc-1")
	state2 := sm.Get("svc-2")

	state1.State = StateUp
	state2.State = StateDown

	// Verify they're independent
	if sm.Get("svc-1").State != StateUp {
		t.Error("svc-1 state should be UP")
	}
	if sm.Get("svc-2").State != StateDown {
		t.Error("svc-2 state should be DOWN")
	}
}

func TestStateManager_WithState_NewService(t *testing.T) {
	sm := NewStateManager()

	var capturedState *ServiceState
	sm.WithState("svc-1", func(st *ServiceState) {
		capturedState = st
		st.State = StateUp
	})

	if capturedState == nil {
		t.Fatal("callback received nil state")
	}
	if capturedState.State != StateUp {
		t.Errorf("state = %q, want %q", capturedState.State, StateUp)
	}

	// Verify change persisted
	state := sm.Get("svc-1")
	if state.State != StateUp {
		t.Error("state change should persist")
	}
}

func TestStateManager_WithState_Modifications(t *testing.T) {
	sm := NewStateManager()

	// First call to set initial state
	sm.WithState("svc-1", func(st *ServiceState) {
		st.State = StateUp
		st.ConsecutiveFailures = 0
	})

	// Second call to modify
	sm.WithState("svc-1", func(st *ServiceState) {
		st.ConsecutiveFailures++
		if st.ConsecutiveFailures >= 3 {
			st.State = StateDown
		}
	})

	// Continue incrementing
	for i := 0; i < 3; i++ {
		sm.WithState("svc-1", func(st *ServiceState) {
			st.ConsecutiveFailures++
			if st.ConsecutiveFailures >= 3 {
				st.State = StateDown
			}
		})
	}

	state := sm.Get("svc-1")
	if state.ConsecutiveFailures != 4 {
		t.Errorf("failures = %d, want 4", state.ConsecutiveFailures)
	}
	if state.State != StateDown {
		t.Errorf("state = %q, want %q", state.State, StateDown)
	}
}

func TestStateManager_Concurrency(t *testing.T) {
	sm := NewStateManager()

	const numGoroutines = 100
	const numIterations = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			serviceID := "svc-1" // All use same service
			for j := 0; j < numIterations; j++ {
				sm.WithState(serviceID, func(st *ServiceState) {
					st.ConsecutiveFailures++
				})
			}
		}(i)
	}

	wg.Wait()

	state := sm.Get("svc-1")
	expected := numGoroutines * numIterations
	if state.ConsecutiveFailures != expected {
		t.Errorf("failures = %d, want %d", state.ConsecutiveFailures, expected)
	}
}

func TestStateManager_ConcurrencyMultipleServices(t *testing.T) {
	sm := NewStateManager()

	const numServices = 10
	const numGoroutines = 10
	const numIterations = 100

	var wg sync.WaitGroup
	wg.Add(numServices * numGoroutines)

	for s := 0; s < numServices; s++ {
		serviceID := "svc-" + string(rune('0'+s))
		for g := 0; g < numGoroutines; g++ {
			go func(svcID string) {
				defer wg.Done()
				for i := 0; i < numIterations; i++ {
					sm.WithState(svcID, func(st *ServiceState) {
						st.ConsecutiveFailures++
					})
				}
			}(serviceID)
		}
	}

	wg.Wait()

	// Verify each service has correct count
	expected := numGoroutines * numIterations
	for s := 0; s < numServices; s++ {
		serviceID := "svc-" + string(rune('0'+s))
		state := sm.Get(serviceID)
		if state.ConsecutiveFailures != expected {
			t.Errorf("service %s: failures = %d, want %d", serviceID, state.ConsecutiveFailures, expected)
		}
	}
}

func TestServiceState_Fields(t *testing.T) {
	now := time.Now()
	state := ServiceState{
		State:               StateDown,
		ConsecutiveFailures: 5,
		LastDownAlertAt:     now,
		DownNotified:        true,
		LastResultAt:        now.Add(-time.Minute),
	}

	if state.State != StateDown {
		t.Errorf("State = %q, want %q", state.State, StateDown)
	}
	if state.ConsecutiveFailures != 5 {
		t.Errorf("ConsecutiveFailures = %d, want 5", state.ConsecutiveFailures)
	}
	if !state.DownNotified {
		t.Error("DownNotified should be true")
	}
	if !state.LastDownAlertAt.Equal(now) {
		t.Error("LastDownAlertAt mismatch")
	}
}

func TestAlertState_Constants(t *testing.T) {
	if StateUnknown != "UNKNOWN" {
		t.Errorf("StateUnknown = %q, want %q", StateUnknown, "UNKNOWN")
	}
	if StateUp != "UP" {
		t.Errorf("StateUp = %q, want %q", StateUp, "UP")
	}
	if StateDown != "DOWN" {
		t.Errorf("StateDown = %q, want %q", StateDown, "DOWN")
	}
}

func TestNewStateManager(t *testing.T) {
	sm := NewStateManager()

	if sm == nil {
		t.Fatal("NewStateManager returned nil")
	}
	if sm.state == nil {
		t.Error("state map is nil")
	}
}

func TestServiceState_Transitions(t *testing.T) {
	sm := NewStateManager()

	// Initial state is UNKNOWN
	state := sm.Get("svc-1")
	if state.State != StateUnknown {
		t.Errorf("initial state = %q, want UNKNOWN", state.State)
	}

	// Transition to UP
	sm.WithState("svc-1", func(st *ServiceState) {
		st.State = StateUp
	})
	if sm.Get("svc-1").State != StateUp {
		t.Error("state should be UP")
	}

	// Transition to DOWN
	sm.WithState("svc-1", func(st *ServiceState) {
		st.State = StateDown
	})
	if sm.Get("svc-1").State != StateDown {
		t.Error("state should be DOWN")
	}

	// Back to UP (recovery)
	sm.WithState("svc-1", func(st *ServiceState) {
		st.State = StateUp
	})
	if sm.Get("svc-1").State != StateUp {
		t.Error("state should be UP after recovery")
	}
}

func TestServiceState_FailureTracking(t *testing.T) {
	sm := NewStateManager()

	// Simulate failure accumulation
	for i := 1; i <= 5; i++ {
		sm.WithState("svc-1", func(st *ServiceState) {
			st.ConsecutiveFailures++
		})

		state := sm.Get("svc-1")
		if state.ConsecutiveFailures != i {
			t.Errorf("after %d failures: count = %d", i, state.ConsecutiveFailures)
		}
	}

	// Reset on success
	sm.WithState("svc-1", func(st *ServiceState) {
		st.ConsecutiveFailures = 0
	})

	state := sm.Get("svc-1")
	if state.ConsecutiveFailures != 0 {
		t.Error("failures should be reset to 0")
	}
}

func TestServiceState_AlertNotificationTracking(t *testing.T) {
	sm := NewStateManager()
	now := time.Now()

	// Simulate DOWN notification
	sm.WithState("svc-1", func(st *ServiceState) {
		st.State = StateDown
		st.DownNotified = true
		st.LastDownAlertAt = now
	})

	state := sm.Get("svc-1")
	if !state.DownNotified {
		t.Error("DownNotified should be true")
	}
	if !state.LastDownAlertAt.Equal(now) {
		t.Error("LastDownAlertAt not set correctly")
	}

	// Simulate recovery - clear notification flag
	sm.WithState("svc-1", func(st *ServiceState) {
		st.State = StateUp
		st.DownNotified = false
	})

	state = sm.Get("svc-1")
	if state.DownNotified {
		t.Error("DownNotified should be cleared after recovery")
	}
}
