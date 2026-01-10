package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"

	"uptiq/internal/checks"
	"uptiq/internal/config"
)

func TestNewBundle(t *testing.T) {
	bundle := NewBundle()

	if bundle == nil {
		t.Fatal("NewBundle returned nil")
	}
	if bundle.Registry == nil {
		t.Error("Registry is nil")
	}
	if bundle.Collector == nil {
		t.Error("Collector is nil")
	}
}

func TestNewBundle_MetricsRegistered(t *testing.T) {
	bundle := NewBundle()

	// Try to gather metrics - this will fail if metrics aren't registered
	families, err := bundle.Registry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	// Should have at least build_info and config_reload_success
	expectedMetrics := []string{
		"uptiq_build_info",
		"uptiq_config_reload_success",
	}

	familyNames := make(map[string]bool)
	for _, f := range families {
		familyNames[f.GetName()] = true
	}

	for _, name := range expectedMetrics {
		if !familyNames[name] {
			t.Errorf("expected metric %s not found", name)
		}
	}
}

func TestCollector_EnsureServices(t *testing.T) {
	bundle := NewBundle()

	services := []config.Service{
		{ID: "svc-1", Name: "Service 1", Type: "http"},
		{ID: "svc-2", Name: "Service 2", Type: "tcp"},
	}

	bundle.Collector.EnsureServices(services)

	// Gather metrics to verify they exist
	families, err := bundle.Registry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	// Find uptiq_up metric
	var upFamily *io_prometheus_client.MetricFamily
	for _, f := range families {
		if f.GetName() == "uptiq_up" {
			upFamily = f
			break
		}
	}

	if upFamily == nil {
		t.Fatal("uptiq_up metric not found")
	}

	// Should have 2 series (one per service)
	if len(upFamily.GetMetric()) != 2 {
		t.Errorf("expected 2 uptiq_up series, got %d", len(upFamily.GetMetric()))
	}
}

func TestCollector_EnsureServices_Idempotent(t *testing.T) {
	bundle := NewBundle()

	services := []config.Service{
		{ID: "svc-1", Name: "Service 1", Type: "http"},
	}

	// Call multiple times
	bundle.Collector.EnsureServices(services)
	bundle.Collector.EnsureServices(services)
	bundle.Collector.EnsureServices(services)

	// Should not panic or create duplicates
	families, err := bundle.Registry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	var upFamily *io_prometheus_client.MetricFamily
	for _, f := range families {
		if f.GetName() == "uptiq_up" {
			upFamily = f
			break
		}
	}

	if upFamily == nil {
		t.Fatal("uptiq_up metric not found")
	}

	// Should still have exactly 1 series
	if len(upFamily.GetMetric()) != 1 {
		t.Errorf("expected 1 uptiq_up series, got %d", len(upFamily.GetMetric()))
	}
}

func TestCollector_Observe_Success(t *testing.T) {
	bundle := NewBundle()

	svc := config.Service{ID: "test", Name: "Test", Type: "http"}
	bundle.Collector.EnsureServices([]config.Service{svc})

	result := checks.Result{
		Success:    true,
		StatusCode: 200,
		Latency:    100 * time.Millisecond,
	}

	bundle.Collector.Observe(svc, result)

	families, err := bundle.Registry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	// Check uptiq_up is 1
	for _, f := range families {
		if f.GetName() == "uptiq_up" {
			for _, m := range f.GetMetric() {
				if m.GetGauge().GetValue() != 1 {
					t.Errorf("uptiq_up = %v, want 1", m.GetGauge().GetValue())
				}
			}
		}
	}

	// Check check_total has success increment
	for _, f := range families {
		if f.GetName() == "uptiq_check_total" {
			found := false
			for _, m := range f.GetMetric() {
				for _, l := range m.GetLabel() {
					if l.GetName() == "result" && l.GetValue() == "success" {
						found = true
						if m.GetCounter().GetValue() != 1 {
							t.Errorf("success counter = %v, want 1", m.GetCounter().GetValue())
						}
					}
				}
			}
			if !found {
				t.Error("success label not found in uptiq_check_total")
			}
		}
	}
}

func TestCollector_Observe_Failure(t *testing.T) {
	bundle := NewBundle()

	svc := config.Service{ID: "test", Name: "Test", Type: "http"}
	bundle.Collector.EnsureServices([]config.Service{svc})

	result := checks.Result{
		Success:    false,
		StatusCode: 500,
		Latency:    200 * time.Millisecond,
		Error:      "internal error",
	}

	bundle.Collector.Observe(svc, result)

	families, err := bundle.Registry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	// Check uptiq_up is 0
	for _, f := range families {
		if f.GetName() == "uptiq_up" {
			for _, m := range f.GetMetric() {
				if m.GetGauge().GetValue() != 0 {
					t.Errorf("uptiq_up = %v, want 0", m.GetGauge().GetValue())
				}
			}
		}
	}
}

func TestCollector_Observe_UpdatesLastSuccess(t *testing.T) {
	bundle := NewBundle()

	svc := config.Service{ID: "test", Name: "Test", Type: "http"}
	bundle.Collector.EnsureServices([]config.Service{svc})

	// First observe (success)
	bundle.Collector.Observe(svc, checks.Result{Success: true, Latency: 50 * time.Millisecond})

	families, err := bundle.Registry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	// Check last_success_timestamp is set
	for _, f := range families {
		if f.GetName() == "uptiq_last_success_timestamp" {
			for _, m := range f.GetMetric() {
				ts := m.GetGauge().GetValue()
				if ts <= 0 {
					t.Errorf("last_success_timestamp = %v, want > 0", ts)
				}
			}
		}
	}
}

func TestCollector_Observe_LatencyHistogram(t *testing.T) {
	bundle := NewBundle()

	svc := config.Service{ID: "test", Name: "Test", Type: "http"}
	bundle.Collector.EnsureServices([]config.Service{svc})

	// Observe multiple results
	for _, latency := range []time.Duration{
		10 * time.Millisecond,
		50 * time.Millisecond,
		100 * time.Millisecond,
		500 * time.Millisecond,
	} {
		bundle.Collector.Observe(svc, checks.Result{Success: true, Latency: latency})
	}

	families, err := bundle.Registry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	// Check histogram has observations
	for _, f := range families {
		if f.GetName() == "uptiq_check_latency_seconds" {
			for _, m := range f.GetMetric() {
				count := m.GetHistogram().GetSampleCount()
				if count != 4 {
					t.Errorf("histogram count = %d, want 4", count)
				}
			}
		}
	}
}

func TestCollector_ConfigReloadSuccess(t *testing.T) {
	bundle := NewBundle()

	// Initial value should be 1 (set in NewBundle)
	families, _ := bundle.Registry.Gather()
	for _, f := range families {
		if f.GetName() == "uptiq_config_reload_success" {
			for _, m := range f.GetMetric() {
				if m.GetGauge().GetValue() != 1 {
					t.Errorf("initial config_reload_success = %v, want 1", m.GetGauge().GetValue())
				}
			}
		}
	}

	// Set to 0 (failure)
	bundle.Collector.ConfigReloadSuccess.Set(0)

	families, _ = bundle.Registry.Gather()
	for _, f := range families {
		if f.GetName() == "uptiq_config_reload_success" {
			for _, m := range f.GetMetric() {
				if m.GetGauge().GetValue() != 0 {
					t.Errorf("config_reload_success = %v, want 0", m.GetGauge().GetValue())
				}
			}
		}
	}
}

func TestCollector_BuildInfo(t *testing.T) {
	bundle := NewBundle()

	families, err := bundle.Registry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	found := false
	for _, f := range families {
		if f.GetName() == "uptiq_build_info" {
			found = true
			for _, m := range f.GetMetric() {
				if m.GetGauge().GetValue() != 1 {
					t.Errorf("build_info value = %v, want 1", m.GetGauge().GetValue())
				}

				// Check labels exist
				labels := make(map[string]string)
				for _, l := range m.GetLabel() {
					labels[l.GetName()] = l.GetValue()
				}

				if labels["go_version"] == "" {
					t.Error("go_version label missing")
				}
				if labels["os"] == "" {
					t.Error("os label missing")
				}
				if labels["arch"] == "" {
					t.Error("arch label missing")
				}
			}
		}
	}

	if !found {
		t.Error("uptiq_build_info metric not found")
	}
}

func TestLabelConstants(t *testing.T) {
	if LabelServiceID != "service_id" {
		t.Errorf("LabelServiceID = %q, want %q", LabelServiceID, "service_id")
	}
	if LabelServiceName != "service_name" {
		t.Errorf("LabelServiceName = %q, want %q", LabelServiceName, "service_name")
	}
	if LabelType != "type" {
		t.Errorf("LabelType = %q, want %q", LabelType, "type")
	}
	if LabelResult != "result" {
		t.Errorf("LabelResult = %q, want %q", LabelResult, "result")
	}
	if ResultSuccess != "success" {
		t.Errorf("ResultSuccess = %q, want %q", ResultSuccess, "success")
	}
	if ResultFailure != "failure" {
		t.Errorf("ResultFailure = %q, want %q", ResultFailure, "failure")
	}
}

func TestCollector_MultipleServices(t *testing.T) {
	bundle := NewBundle()

	services := []config.Service{
		{ID: "web", Name: "Web Server", Type: "http"},
		{ID: "api", Name: "API Server", Type: "http"},
		{ID: "db", Name: "Database", Type: "tcp"},
	}

	bundle.Collector.EnsureServices(services)

	// Observe different results for each
	bundle.Collector.Observe(services[0], checks.Result{Success: true, Latency: 50 * time.Millisecond})
	bundle.Collector.Observe(services[1], checks.Result{Success: false, Latency: 100 * time.Millisecond})
	bundle.Collector.Observe(services[2], checks.Result{Success: true, Latency: 10 * time.Millisecond})

	families, err := bundle.Registry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	// Check uptiq_up has correct values
	upValues := make(map[string]float64)
	for _, f := range families {
		if f.GetName() == "uptiq_up" {
			for _, m := range f.GetMetric() {
				var serviceID string
				for _, l := range m.GetLabel() {
					if l.GetName() == "service_id" {
						serviceID = l.GetValue()
					}
				}
				upValues[serviceID] = m.GetGauge().GetValue()
			}
		}
	}

	if upValues["web"] != 1 {
		t.Errorf("web service up = %v, want 1", upValues["web"])
	}
	if upValues["api"] != 0 {
		t.Errorf("api service up = %v, want 0", upValues["api"])
	}
	if upValues["db"] != 1 {
		t.Errorf("db service up = %v, want 1", upValues["db"])
	}
}

func TestNewCollector(t *testing.T) {
	col := newCollector()

	if col == nil {
		t.Fatal("newCollector returned nil")
	}
	if col.CheckTotal == nil {
		t.Error("CheckTotal is nil")
	}
	if col.CheckLatencySeconds == nil {
		t.Error("CheckLatencySeconds is nil")
	}
	if col.Up == nil {
		t.Error("Up is nil")
	}
	if col.LastSuccessTimestamp == nil {
		t.Error("LastSuccessTimestamp is nil")
	}
	if col.BuildInfo == nil {
		t.Error("BuildInfo is nil")
	}
	if col.ConfigReloadSuccess == nil {
		t.Error("ConfigReloadSuccess is nil")
	}
	if col.initialized == nil {
		t.Error("initialized map is nil")
	}
}

func TestBundle_RegistryGather(t *testing.T) {
	bundle := NewBundle()

	// Should be able to gather without error
	families, err := bundle.Registry.Gather()
	if err != nil {
		t.Fatalf("Gather() error: %v", err)
	}

	if len(families) == 0 {
		t.Error("expected at least some metrics")
	}
}

func TestCollector_ThreadSafety(t *testing.T) {
	bundle := NewBundle()

	services := []config.Service{
		{ID: "svc", Name: "Service", Type: "http"},
	}

	bundle.Collector.EnsureServices(services)

	// Concurrent observations
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				bundle.Collector.Observe(services[0], checks.Result{
					Success: j%2 == 0,
					Latency: time.Duration(j) * time.Millisecond,
				})
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic and metrics should be accessible
	families, err := bundle.Registry.Gather()
	if err != nil {
		t.Fatalf("Gather() error after concurrent access: %v", err)
	}

	if len(families) == 0 {
		t.Error("expected metrics after concurrent access")
	}
}

func TestCollector_MetricNames(t *testing.T) {
	bundle := NewBundle()

	services := []config.Service{
		{ID: "svc", Name: "Service", Type: "http"},
	}
	bundle.Collector.EnsureServices(services)
	bundle.Collector.Observe(services[0], checks.Result{Success: true, Latency: 50 * time.Millisecond})

	families, _ := bundle.Registry.Gather()

	expectedNames := []string{
		"uptiq_check_total",
		"uptiq_check_latency_seconds",
		"uptiq_up",
		"uptiq_last_success_timestamp",
		"uptiq_build_info",
		"uptiq_config_reload_success",
	}

	familyNames := make(map[string]bool)
	for _, f := range families {
		familyNames[f.GetName()] = true
	}

	for _, name := range expectedNames {
		if !familyNames[name] {
			t.Errorf("expected metric %s not found", name)
		}
	}
}

func TestCollector_RegistrationPanic(t *testing.T) {
	// Test that creating multiple bundles with same metrics names
	// doesn't cause registration panic (each has its own registry)
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("unexpected panic: %v", r)
		}
	}()

	bundle1 := NewBundle()
	bundle2 := NewBundle()

	if bundle1 == bundle2 {
		t.Error("bundles should be independent")
	}
}

func TestCollector_DefaultPrometheusRegistry(t *testing.T) {
	// Verify we're not accidentally registering to default registry
	// by checking our custom registry is separate
	bundle := NewBundle()

	// Our registry should not be the default
	if bundle.Registry == prometheus.DefaultRegisterer.(*prometheus.Registry) {
		t.Error("should use custom registry, not default")
	}
}

func TestCollector_MetricHelp(t *testing.T) {
	bundle := NewBundle()

	services := []config.Service{
		{ID: "svc", Name: "Service", Type: "http"},
	}
	bundle.Collector.EnsureServices(services)

	families, _ := bundle.Registry.Gather()

	// All metrics should have help text
	for _, f := range families {
		if f.GetHelp() == "" {
			t.Errorf("metric %s has no help text", f.GetName())
		}
		if !strings.HasPrefix(f.GetName(), "uptiq_") {
			t.Errorf("metric %s should start with uptiq_", f.GetName())
		}
	}
}
