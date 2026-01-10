package config

import "testing"

func TestApplyGlobalDefaults(t *testing.T) {
	tests := []struct {
		name   string
		input  GlobalConfig
		expect GlobalConfig
	}{
		{
			name:  "all empty",
			input: GlobalConfig{},
			expect: GlobalConfig{
				ScrapeBind:      DefaultScrapeBind,
				LogLevel:        DefaultLogLevel,
				DefaultTimeout:  DefaultTimeout,
				DefaultInterval: DefaultInterval,
				WorkerCount:     DefaultWorkerCount,
				Jitter:          DefaultJitter,
			},
		},
		{
			name: "partial config",
			input: GlobalConfig{
				ScrapeBind: "127.0.0.1:9090",
				LogLevel:   "debug",
			},
			expect: GlobalConfig{
				ScrapeBind:      "127.0.0.1:9090",
				LogLevel:        "debug",
				DefaultTimeout:  DefaultTimeout,
				DefaultInterval: DefaultInterval,
				WorkerCount:     DefaultWorkerCount,
				Jitter:          DefaultJitter,
			},
		},
		{
			name: "full config preserved",
			input: GlobalConfig{
				ScrapeBind:      "0.0.0.0:3000",
				LogLevel:        "warn",
				DefaultTimeout:  "10s",
				DefaultInterval: "1m",
				WorkerCount:     20,
				Jitter:          "1s",
			},
			expect: GlobalConfig{
				ScrapeBind:      "0.0.0.0:3000",
				LogLevel:        "warn",
				DefaultTimeout:  "10s",
				DefaultInterval: "1m",
				WorkerCount:     20,
				Jitter:          "1s",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			global := tc.input
			applyGlobalDefaults(&global)

			if global.ScrapeBind != tc.expect.ScrapeBind {
				t.Errorf("ScrapeBind = %q, want %q", global.ScrapeBind, tc.expect.ScrapeBind)
			}
			if global.LogLevel != tc.expect.LogLevel {
				t.Errorf("LogLevel = %q, want %q", global.LogLevel, tc.expect.LogLevel)
			}
			if global.DefaultTimeout != tc.expect.DefaultTimeout {
				t.Errorf("DefaultTimeout = %q, want %q", global.DefaultTimeout, tc.expect.DefaultTimeout)
			}
			if global.DefaultInterval != tc.expect.DefaultInterval {
				t.Errorf("DefaultInterval = %q, want %q", global.DefaultInterval, tc.expect.DefaultInterval)
			}
			if global.WorkerCount != tc.expect.WorkerCount {
				t.Errorf("WorkerCount = %d, want %d", global.WorkerCount, tc.expect.WorkerCount)
			}
			if global.Jitter != tc.expect.Jitter {
				t.Errorf("Jitter = %q, want %q", global.Jitter, tc.expect.Jitter)
			}
		})
	}
}

func TestApplyServiceDefaults(t *testing.T) {
	cfg := &Config{
		Global: GlobalConfig{
			DefaultTimeout:  "10s",
			DefaultInterval: "1m",
		},
		Services: []Service{
			{
				ID:       "svc-1",
				Type:     "http",
				Timeout:  "",
				Interval: "",
				Method:   "",
			},
			{
				ID:       "svc-2",
				Type:     "http",
				Timeout:  "5s",
				Interval: "30s",
				Method:   "POST",
			},
			{
				ID:       "svc-3",
				Type:     "tcp",
				Timeout:  "",
				Interval: "",
			},
		},
	}

	applyServiceDefaults(cfg)

	// Service 1: should get all defaults
	if cfg.Services[0].Timeout != "10s" {
		t.Errorf("svc-1 Timeout = %q, want %q", cfg.Services[0].Timeout, "10s")
	}
	if cfg.Services[0].Interval != "1m" {
		t.Errorf("svc-1 Interval = %q, want %q", cfg.Services[0].Interval, "1m")
	}
	if cfg.Services[0].Method != DefaultHTTPMethod {
		t.Errorf("svc-1 Method = %q, want %q", cfg.Services[0].Method, DefaultHTTPMethod)
	}

	// Service 2: should keep existing values
	if cfg.Services[1].Timeout != "5s" {
		t.Errorf("svc-2 Timeout = %q, want %q", cfg.Services[1].Timeout, "5s")
	}
	if cfg.Services[1].Interval != "30s" {
		t.Errorf("svc-2 Interval = %q, want %q", cfg.Services[1].Interval, "30s")
	}
	if cfg.Services[1].Method != "POST" {
		t.Errorf("svc-2 Method = %q, want %q", cfg.Services[1].Method, "POST")
	}

	// Service 3: TCP should get timeout/interval defaults but no method
	if cfg.Services[2].Timeout != "10s" {
		t.Errorf("svc-3 Timeout = %q, want %q", cfg.Services[2].Timeout, "10s")
	}
	if cfg.Services[2].Interval != "1m" {
		t.Errorf("svc-3 Interval = %q, want %q", cfg.Services[2].Interval, "1m")
	}
	if cfg.Services[2].Method != "" {
		t.Errorf("svc-3 Method = %q, want empty (TCP service)", cfg.Services[2].Method)
	}
}

func TestApplyDefaults_Integration(t *testing.T) {
	cfg := &Config{
		Global:   GlobalConfig{},
		Services: []Service{{ID: "test", Type: "http"}},
	}

	applyDefaults(cfg)

	// Verify global defaults
	if cfg.Global.ScrapeBind != DefaultScrapeBind {
		t.Errorf("Global ScrapeBind = %q, want %q", cfg.Global.ScrapeBind, DefaultScrapeBind)
	}

	// Verify service defaults inherited from global
	if cfg.Services[0].Timeout != DefaultTimeout {
		t.Errorf("Service Timeout = %q, want %q", cfg.Services[0].Timeout, DefaultTimeout)
	}
	if cfg.Services[0].Interval != DefaultInterval {
		t.Errorf("Service Interval = %q, want %q", cfg.Services[0].Interval, DefaultInterval)
	}
}

func TestDefaultConstants(t *testing.T) {
	// Verify default constants are reasonable
	if DefaultScrapeBind == "" {
		t.Error("DefaultScrapeBind should not be empty")
	}
	if DefaultLogLevel == "" {
		t.Error("DefaultLogLevel should not be empty")
	}
	if DefaultTimeout == "" {
		t.Error("DefaultTimeout should not be empty")
	}
	if DefaultInterval == "" {
		t.Error("DefaultInterval should not be empty")
	}
	if DefaultWorkerCount < MinWorkerCount || DefaultWorkerCount > MaxWorkerCount {
		t.Errorf("DefaultWorkerCount (%d) should be between %d and %d",
			DefaultWorkerCount, MinWorkerCount, MaxWorkerCount)
	}
	if MinPort <= 0 || MaxPort > 65535 {
		t.Errorf("Port range invalid: min=%d, max=%d", MinPort, MaxPort)
	}
}
