package alerting

import (
	"testing"
	"time"

	"uptiq/internal/config"
)

func TestRouter_Resolve_SingleRoute(t *testing.T) {
	cfg := config.AlertingConfig{
		Routes: []config.Route{
			{
				Match: config.RouteMatch{
					ServiceIDs: []string{"svc-1", "svc-2"},
				},
				Policy: config.RoutePolicy{
					FailureThreshold: 3,
					Cooldown:         "5m",
					RecoveryAlert:    true,
				},
				Notify: []string{"discord", "slack"},
			},
		},
	}

	router := NewRouter(cfg)

	// Test matching service
	route := router.Resolve("svc-1")
	if !route.Valid {
		t.Error("expected valid route for svc-1")
	}
	if len(route.Channels) != 2 {
		t.Errorf("expected 2 channels, got %d", len(route.Channels))
	}
	if route.Policy.FailureThreshold != 3 {
		t.Errorf("FailureThreshold = %d, want 3", route.Policy.FailureThreshold)
	}
	if route.Policy.Cooldown != 5*time.Minute {
		t.Errorf("Cooldown = %v, want 5m", route.Policy.Cooldown)
	}
	if !route.Policy.RecoveryAlert {
		t.Error("RecoveryAlert should be true")
	}

	// Test non-matching service
	route = router.Resolve("svc-unknown")
	if route.Valid {
		t.Error("expected invalid route for unknown service")
	}
}

func TestRouter_Resolve_MultipleRoutes(t *testing.T) {
	cfg := config.AlertingConfig{
		Routes: []config.Route{
			{
				Match:  config.RouteMatch{ServiceIDs: []string{"svc-1"}},
				Policy: config.RoutePolicy{FailureThreshold: 2, Cooldown: "5m"},
				Notify: []string{"discord"},
			},
			{
				Match:  config.RouteMatch{ServiceIDs: []string{"svc-1"}},
				Policy: config.RoutePolicy{FailureThreshold: 5, Cooldown: "10m", RecoveryAlert: true},
				Notify: []string{"slack"},
			},
		},
	}

	router := NewRouter(cfg)
	route := router.Resolve("svc-1")

	if !route.Valid {
		t.Error("expected valid route")
	}

	// Should have channels from both routes (deduplicated)
	if len(route.Channels) != 2 {
		t.Errorf("expected 2 channels, got %d: %v", len(route.Channels), route.Channels)
	}

	// Policy should be merged (max values)
	if route.Policy.FailureThreshold != 5 {
		t.Errorf("FailureThreshold = %d, want 5 (max)", route.Policy.FailureThreshold)
	}
	if route.Policy.Cooldown != 10*time.Minute {
		t.Errorf("Cooldown = %v, want 10m (max)", route.Policy.Cooldown)
	}
	if !route.Policy.RecoveryAlert {
		t.Error("RecoveryAlert should be true (any true)")
	}
}

func TestRouter_Resolve_DuplicateChannels(t *testing.T) {
	cfg := config.AlertingConfig{
		Routes: []config.Route{
			{
				Match:  config.RouteMatch{ServiceIDs: []string{"svc-1"}},
				Notify: []string{"discord", "slack"},
			},
			{
				Match:  config.RouteMatch{ServiceIDs: []string{"svc-1"}},
				Notify: []string{"slack", "email"}, // slack is duplicate
			},
		},
	}

	router := NewRouter(cfg)
	route := router.Resolve("svc-1")

	// Should deduplicate channels
	if len(route.Channels) != 3 {
		t.Errorf("expected 3 unique channels, got %d: %v", len(route.Channels), route.Channels)
	}
}

func TestRouter_Resolve_NoChannels(t *testing.T) {
	cfg := config.AlertingConfig{
		Routes: []config.Route{
			{
				Match:  config.RouteMatch{ServiceIDs: []string{"svc-1"}},
				Notify: []string{}, // No channels
			},
		},
	}

	router := NewRouter(cfg)
	route := router.Resolve("svc-1")

	if route.Valid {
		t.Error("expected invalid route when no channels")
	}
}

func TestRouter_Resolve_EmptyServiceIDs(t *testing.T) {
	cfg := config.AlertingConfig{
		Routes: []config.Route{
			{
				Match:  config.RouteMatch{ServiceIDs: []string{}},
				Notify: []string{"discord"},
			},
		},
	}

	router := NewRouter(cfg)
	route := router.Resolve("any-service")

	if route.Valid {
		t.Error("expected invalid route when no service IDs match")
	}
}

func TestRouter_Resolve_WhitespaceHandling(t *testing.T) {
	cfg := config.AlertingConfig{
		Routes: []config.Route{
			{
				Match:  config.RouteMatch{ServiceIDs: []string{"  svc-1  ", "svc-2", "  "}},
				Notify: []string{"  discord  ", "slack"},
			},
		},
	}

	router := NewRouter(cfg)

	// Trimmed service ID should match
	route := router.Resolve("svc-1")
	if !route.Valid {
		t.Error("expected valid route for trimmed service ID")
	}

	// Channels should be trimmed
	found := false
	for _, ch := range route.Channels {
		if ch == "discord" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected trimmed channel 'discord' in %v", route.Channels)
	}
}

func TestCompilePolicy_Defaults(t *testing.T) {
	policy := compilePolicy(config.RoutePolicy{})

	if policy.FailureThreshold != 1 {
		t.Errorf("default FailureThreshold = %d, want 1", policy.FailureThreshold)
	}
	if policy.Cooldown != 0 {
		t.Errorf("default Cooldown = %v, want 0", policy.Cooldown)
	}
	if policy.RecoveryAlert {
		t.Error("default RecoveryAlert should be false")
	}
}

func TestCompilePolicy_ValidValues(t *testing.T) {
	policy := compilePolicy(config.RoutePolicy{
		FailureThreshold: 5,
		Cooldown:         "10m",
		RecoveryAlert:    true,
	})

	if policy.FailureThreshold != 5 {
		t.Errorf("FailureThreshold = %d, want 5", policy.FailureThreshold)
	}
	if policy.Cooldown != 10*time.Minute {
		t.Errorf("Cooldown = %v, want 10m", policy.Cooldown)
	}
	if !policy.RecoveryAlert {
		t.Error("RecoveryAlert should be true")
	}
}

func TestCompilePolicy_InvalidCooldown(t *testing.T) {
	policy := compilePolicy(config.RoutePolicy{
		Cooldown: "invalid",
	})

	if policy.Cooldown != 0 {
		t.Errorf("invalid cooldown should default to 0, got %v", policy.Cooldown)
	}
}

func TestCompilePolicy_NegativeCooldown(t *testing.T) {
	policy := compilePolicy(config.RoutePolicy{
		Cooldown: "-5m",
	})

	if policy.Cooldown != 0 {
		t.Errorf("negative cooldown should default to 0, got %v", policy.Cooldown)
	}
}

func TestCompilePolicy_ZeroThreshold(t *testing.T) {
	policy := compilePolicy(config.RoutePolicy{
		FailureThreshold: 0,
	})

	// Zero should default to 1
	if policy.FailureThreshold != 1 {
		t.Errorf("zero threshold should default to 1, got %d", policy.FailureThreshold)
	}
}

func TestMergePolicy(t *testing.T) {
	tests := []struct {
		name   string
		base   ResolvedPolicy
		other  ResolvedPolicy
		expect ResolvedPolicy
	}{
		{
			name:   "other has higher threshold",
			base:   ResolvedPolicy{FailureThreshold: 2, Cooldown: 5 * time.Minute},
			other:  ResolvedPolicy{FailureThreshold: 5, Cooldown: 3 * time.Minute},
			expect: ResolvedPolicy{FailureThreshold: 5, Cooldown: 5 * time.Minute},
		},
		{
			name:   "other has higher cooldown",
			base:   ResolvedPolicy{FailureThreshold: 3, Cooldown: 2 * time.Minute},
			other:  ResolvedPolicy{FailureThreshold: 2, Cooldown: 10 * time.Minute},
			expect: ResolvedPolicy{FailureThreshold: 3, Cooldown: 10 * time.Minute},
		},
		{
			name:   "recovery alert true wins",
			base:   ResolvedPolicy{RecoveryAlert: false},
			other:  ResolvedPolicy{RecoveryAlert: true},
			expect: ResolvedPolicy{FailureThreshold: 0, RecoveryAlert: true},
		},
		{
			name:   "base recovery alert preserved",
			base:   ResolvedPolicy{RecoveryAlert: true},
			other:  ResolvedPolicy{RecoveryAlert: false},
			expect: ResolvedPolicy{RecoveryAlert: true},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := mergePolicy(tc.base, tc.other)

			if result.FailureThreshold != tc.expect.FailureThreshold {
				t.Errorf("FailureThreshold = %d, want %d", result.FailureThreshold, tc.expect.FailureThreshold)
			}
			if result.Cooldown != tc.expect.Cooldown {
				t.Errorf("Cooldown = %v, want %v", result.Cooldown, tc.expect.Cooldown)
			}
			if result.RecoveryAlert != tc.expect.RecoveryAlert {
				t.Errorf("RecoveryAlert = %v, want %v", result.RecoveryAlert, tc.expect.RecoveryAlert)
			}
		})
	}
}

func TestCleanStrings(t *testing.T) {
	tests := []struct {
		name   string
		input  []string
		expect []string
	}{
		{
			name:   "normal strings",
			input:  []string{"a", "b", "c"},
			expect: []string{"a", "b", "c"},
		},
		{
			name:   "with whitespace",
			input:  []string{"  a  ", "b", "  c"},
			expect: []string{"a", "b", "c"},
		},
		{
			name:   "with empty strings",
			input:  []string{"a", "", "b", "  ", "c"},
			expect: []string{"a", "b", "c"},
		},
		{
			name:   "all empty",
			input:  []string{"", "  ", "   "},
			expect: nil,
		},
		{
			name:   "nil input",
			input:  nil,
			expect: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := cleanStrings(tc.input)

			if len(result) != len(tc.expect) {
				t.Errorf("len = %d, want %d", len(result), len(tc.expect))
			}

			for i, v := range result {
				if v != tc.expect[i] {
					t.Errorf("result[%d] = %q, want %q", i, v, tc.expect[i])
				}
			}
		})
	}
}

func TestNewRouter(t *testing.T) {
	cfg := config.AlertingConfig{
		Routes: []config.Route{
			{
				Match:  config.RouteMatch{ServiceIDs: []string{"svc-1"}},
				Notify: []string{"discord"},
			},
		},
	}

	router := NewRouter(cfg)

	if router == nil {
		t.Fatal("NewRouter returned nil")
	}
	if router.routeIndex == nil {
		t.Error("routeIndex is nil")
	}
}

func TestResolvedRoute_Fields(t *testing.T) {
	route := ResolvedRoute{
		Channels: []string{"discord", "slack"},
		Policy: ResolvedPolicy{
			FailureThreshold: 3,
			Cooldown:         5 * time.Minute,
			RecoveryAlert:    true,
		},
		Valid: true,
	}

	if !route.Valid {
		t.Error("Valid should be true")
	}
	if len(route.Channels) != 2 {
		t.Errorf("Channels len = %d, want 2", len(route.Channels))
	}
}
