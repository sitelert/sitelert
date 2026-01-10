package scheduler

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"uptiq/internal/checks"
	"uptiq/internal/config"
	"uptiq/internal/metrics"
)

type mockResultHandler struct {
	results []checks.Result
	count   int32
}

func (h *mockResultHandler) HandleResult(svc config.Service, res checks.Result) {
	h.results = append(h.results, res)
	atomic.AddInt32(&h.count, 1)
}

func TestNew(t *testing.T) {
	cfg := &config.Config{
		Global: config.GlobalConfig{
			WorkerCount: 5,
			Jitter:      "100ms",
		},
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	sched, err := New(cfg, log, nil, nil)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if sched == nil {
		t.Fatal("New() returned nil")
	}
	if sched.workerCount != 5 {
		t.Errorf("workerCount = %d, want 5", sched.workerCount)
	}
	if sched.jitter != 100*time.Millisecond {
		t.Errorf("jitter = %v, want 100ms", sched.jitter)
	}
}

func TestNew_InvalidJitter(t *testing.T) {
	cfg := &config.Config{
		Global: config.GlobalConfig{
			Jitter: "invalid",
		},
	}

	_, err := New(cfg, nil, nil, nil)
	if err == nil {
		t.Error("expected error for invalid jitter")
	}
}

func TestNew_DefaultWorkerCount(t *testing.T) {
	cfg := &config.Config{
		Global: config.GlobalConfig{
			WorkerCount: 0, // Below minimum
			Jitter:      "0s",
		},
	}

	sched, err := New(cfg, nil, nil, nil)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if sched.workerCount < 1 {
		t.Errorf("workerCount = %d, should be at least 1", sched.workerCount)
	}
}

func TestScheduler_StartAndStop(t *testing.T) {
	cfg := &config.Config{
		Global: config.GlobalConfig{
			WorkerCount: 2,
			Jitter:      "0s",
		},
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	sched, err := New(cfg, log, nil, nil)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err = sched.Start(ctx, nil)
	if err != nil {
		t.Errorf("Start() error: %v", err)
	}
}

func TestScheduler_ExecutesChecks(t *testing.T) {
	// Create test HTTP server
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &config.Config{
		Global: config.GlobalConfig{
			WorkerCount: 2,
			Jitter:      "0s",
		},
	}

	services := []config.Service{
		{
			ID:       "test-svc",
			Name:     "Test Service",
			Type:     "http",
			URL:      server.URL,
			Interval: "50ms",
			Timeout:  "1s",
		},
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	m := metrics.NewBundle()
	handler := &mockResultHandler{}

	sched, err := New(cfg, log, m.Collector, handler)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err = sched.Start(ctx, services)
	if err != nil {
		t.Errorf("Start() error: %v", err)
	}

	// Should have made at least a few requests
	count := atomic.LoadInt32(&requestCount)
	if count < 1 {
		t.Errorf("expected at least 1 request, got %d", count)
	}
}

func TestScheduler_UpdateServices(t *testing.T) {
	cfg := &config.Config{
		Global: config.GlobalConfig{
			WorkerCount: 2,
			Jitter:      "0s",
		},
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	sched, err := New(cfg, log, nil, nil)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	// Start with one service
	services := []config.Service{
		{ID: "svc-1", Type: "http", URL: "http://example.com", Interval: "1h", Timeout: "5s"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go func() {
		time.Sleep(50 * time.Millisecond)
		// Update with new services
		newServices := []config.Service{
			{ID: "svc-2", Type: "http", URL: "http://example.org", Interval: "1h", Timeout: "5s"},
			{ID: "svc-3", Type: "tcp", Host: "localhost", Port: 80, Interval: "1h", Timeout: "5s"},
		}
		sched.UpdateServices(newServices)
	}()

	err = sched.Start(ctx, services)
	if err != nil {
		t.Errorf("Start() error: %v", err)
	}
}

func TestTargetForService(t *testing.T) {
	tests := []struct {
		name    string
		service config.Service
		expect  string
	}{
		{
			name:    "http service",
			service: config.Service{Type: "http", URL: "https://api.example.com/health"},
			expect:  "https://api.example.com/health",
		},
		{
			name:    "tcp service",
			service: config.Service{Type: "tcp", Host: "db.local", Port: 5432},
			expect:  "db.local:5432",
		},
		{
			name:    "unknown type",
			service: config.Service{Type: "grpc"},
			expect:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := targetForService(tc.service)
			if got != tc.expect {
				t.Errorf("targetForService() = %q, want %q", got, tc.expect)
			}
		})
	}
}

func TestParseIntervalOrDefault(t *testing.T) {
	tests := []struct {
		input  string
		expect time.Duration
	}{
		{"30s", 30 * time.Second},
		{"1m", 1 * time.Minute},
		{"5m30s", 5*time.Minute + 30*time.Second},
		{"invalid", defaultInterval},
		{"", defaultInterval},
		{"-5s", defaultInterval}, // Negative
		{"0s", defaultInterval},  // Zero
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := parseIntervalOrDefault(tc.input)
			if got != tc.expect {
				t.Errorf("parseIntervalOrDefault(%q) = %v, want %v", tc.input, got, tc.expect)
			}
		})
	}
}

func TestParseTimeoutOrDefault(t *testing.T) {
	tests := []struct {
		input  string
		expect time.Duration
	}{
		{"5s", 5 * time.Second},
		{"500ms", 500 * time.Millisecond},
		{"1m", 1 * time.Minute},
		{"invalid", defaultTimeout},
		{"", defaultTimeout},
		{"-1s", defaultTimeout},
		{"0s", defaultTimeout},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := parseTimeoutOrDefault(tc.input)
			if got != tc.expect {
				t.Errorf("parseTimeoutOrDefault(%q) = %v, want %v", tc.input, got, tc.expect)
			}
		})
	}
}

func TestServicesEqual(t *testing.T) {
	tests := []struct {
		name   string
		a, b   config.Service
		expect bool
	}{
		{
			name:   "identical services",
			a:      config.Service{ID: "svc", Type: "http", URL: "http://example.com", Interval: "30s", Timeout: "5s"},
			b:      config.Service{ID: "svc", Type: "http", URL: "http://example.com", Interval: "30s", Timeout: "5s"},
			expect: true,
		},
		{
			name:   "different ID",
			a:      config.Service{ID: "svc-1", Type: "http", URL: "http://example.com"},
			b:      config.Service{ID: "svc-2", Type: "http", URL: "http://example.com"},
			expect: false,
		},
		{
			name:   "different type",
			a:      config.Service{ID: "svc", Type: "http", URL: "http://example.com"},
			b:      config.Service{ID: "svc", Type: "tcp", Host: "example.com", Port: 80},
			expect: false,
		},
		{
			name:   "different interval",
			a:      config.Service{ID: "svc", Type: "http", URL: "http://example.com", Interval: "30s"},
			b:      config.Service{ID: "svc", Type: "http", URL: "http://example.com", Interval: "1m"},
			expect: false,
		},
		{
			name:   "different URL",
			a:      config.Service{ID: "svc", Type: "http", URL: "http://example.com"},
			b:      config.Service{ID: "svc", Type: "http", URL: "http://example.org"},
			expect: false,
		},
		{
			name:   "different headers",
			a:      config.Service{ID: "svc", Type: "http", URL: "http://example.com", Headers: map[string]string{"A": "1"}},
			b:      config.Service{ID: "svc", Type: "http", URL: "http://example.com", Headers: map[string]string{"B": "2"}},
			expect: false,
		},
		{
			name:   "same headers",
			a:      config.Service{ID: "svc", Type: "http", URL: "http://example.com", Headers: map[string]string{"A": "1"}},
			b:      config.Service{ID: "svc", Type: "http", URL: "http://example.com", Headers: map[string]string{"A": "1"}},
			expect: true,
		},
		{
			name:   "different expected status",
			a:      config.Service{ID: "svc", Type: "http", URL: "http://example.com", ExpectedStatus: []int{200}},
			b:      config.Service{ID: "svc", Type: "http", URL: "http://example.com", ExpectedStatus: []int{200, 201}},
			expect: false,
		},
		{
			name:   "tcp different port",
			a:      config.Service{ID: "svc", Type: "tcp", Host: "localhost", Port: 5432},
			b:      config.Service{ID: "svc", Type: "tcp", Host: "localhost", Port: 3306},
			expect: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := servicesEqual(tc.a, tc.b)
			if got != tc.expect {
				t.Errorf("servicesEqual() = %v, want %v", got, tc.expect)
			}
		})
	}
}

func TestMapsEqual(t *testing.T) {
	tests := []struct {
		name   string
		a, b   map[string]string
		expect bool
	}{
		{
			name:   "both nil",
			a:      nil,
			b:      nil,
			expect: true,
		},
		{
			name:   "empty maps",
			a:      map[string]string{},
			b:      map[string]string{},
			expect: true,
		},
		{
			name:   "equal maps",
			a:      map[string]string{"a": "1", "b": "2"},
			b:      map[string]string{"a": "1", "b": "2"},
			expect: true,
		},
		{
			name:   "different values",
			a:      map[string]string{"a": "1"},
			b:      map[string]string{"a": "2"},
			expect: false,
		},
		{
			name:   "different keys",
			a:      map[string]string{"a": "1"},
			b:      map[string]string{"b": "1"},
			expect: false,
		},
		{
			name:   "different lengths",
			a:      map[string]string{"a": "1"},
			b:      map[string]string{"a": "1", "b": "2"},
			expect: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mapsEqual(tc.a, tc.b)
			if got != tc.expect {
				t.Errorf("mapsEqual() = %v, want %v", got, tc.expect)
			}
		})
	}
}

func TestScheduler_TCPChecks(t *testing.T) {
	// Start a test TCP server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start TCP server: %v", err)
	}
	defer func() { _ = listener.Close() }()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	addr := listener.Addr().(*net.TCPAddr)

	cfg := &config.Config{
		Global: config.GlobalConfig{
			WorkerCount: 1,
			Jitter:      "0s",
		},
	}

	services := []config.Service{
		{
			ID:       "tcp-svc",
			Name:     "TCP Service",
			Type:     "tcp",
			Host:     "127.0.0.1",
			Port:     addr.Port,
			Interval: "50ms",
			Timeout:  "1s",
		},
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	handler := &mockResultHandler{}

	sched, err := New(cfg, log, nil, handler)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err = sched.Start(ctx, services)
	if err != nil {
		t.Errorf("Start() error: %v", err)
	}

	count := atomic.LoadInt32(&handler.count)
	if count < 1 {
		t.Errorf("expected at least 1 TCP check, got %d", count)
	}
}

func TestResetTimer(t *testing.T) {
	timer := time.NewTimer(1 * time.Hour)

	// Reset to shorter duration
	resetTimer(timer, 10*time.Millisecond)

	select {
	case <-timer.C:
		// Timer fired as expected
	case <-time.After(100 * time.Millisecond):
		t.Error("timer did not fire after reset")
	}
}

func TestScheduler_RandomJitter(t *testing.T) {
	cfg := &config.Config{
		Global: config.GlobalConfig{
			WorkerCount: 1,
			Jitter:      "100ms",
		},
	}

	sched, err := New(cfg, nil, nil, nil)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	// Generate multiple jitter values and verify they're in range
	for i := 0; i < 100; i++ {
		jitter := sched.randomJitter()
		if jitter < 0 || jitter > 100*time.Millisecond {
			t.Errorf("jitter %v out of range [0, 100ms]", jitter)
		}
	}
}

func TestScheduler_ZeroJitter(t *testing.T) {
	cfg := &config.Config{
		Global: config.GlobalConfig{
			WorkerCount: 1,
			Jitter:      "0s",
		},
	}

	sched, err := New(cfg, nil, nil, nil)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	for i := 0; i < 10; i++ {
		jitter := sched.randomJitter()
		if jitter != 0 {
			t.Errorf("expected 0 jitter, got %v", jitter)
		}
	}
}
