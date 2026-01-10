package config

import (
	"strings"
	"testing"
)

func TestValidateGlobal_ValidConfig(t *testing.T) {
	cfg := &Config{
		Global: GlobalConfig{
			ScrapeBind:      "0.0.0.0:8080",
			LogLevel:        "info",
			DefaultTimeout:  "5s",
			DefaultInterval: "30s",
			WorkerCount:     10,
			Jitter:          "500ms",
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid config, got error: %v", err)
	}
}

func TestValidateGlobal_InvalidScrapeBind(t *testing.T) {
	tests := []struct {
		name       string
		scrapeBind string
	}{
		{"missing port", "0.0.0.0"},
		{"empty", ""},
		{"invalid format", "not-a-host-port"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				Global: GlobalConfig{
					ScrapeBind:      tc.scrapeBind,
					LogLevel:        "info",
					DefaultTimeout:  "5s",
					DefaultInterval: "30s",
					WorkerCount:     10,
					Jitter:          "0s",
				},
			}

			err := cfg.Validate()
			if err == nil {
				t.Error("expected validation error for invalid scrape_bind")
			}
			if !strings.Contains(err.Error(), "scrape_bind") {
				t.Errorf("expected error to mention scrape_bind, got: %v", err)
			}
		})

	}
}

func TestValidateGlobal_InvalidDurations(t *testing.T) {
	tests := []struct {
		name     string
		timeout  string
		interval string
		jitter   string
		errField string
	}{
		{"invalid timeout", "not-a-duration", "30s", "0s", "default_timeout"},
		{"invalid interval", "5s", "invalid", "0s", "default_interval"},
		{"invalid jitter", "5s", "30s", "bad", "jitter"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				Global: GlobalConfig{
					ScrapeBind:      "0.0.0.0:8080",
					LogLevel:        "info",
					DefaultTimeout:  tc.timeout,
					DefaultInterval: tc.interval,
					WorkerCount:     10,
					Jitter:          tc.jitter,
				},
			}

			err := cfg.Validate()
			if err == nil {
				t.Errorf("expected validation error for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.errField) {
				t.Errorf("error should mention %s: %v", tc.errField, err)
			}
		})
	}
}

func TestValidateGlobal_WorkerCountBounds(t *testing.T) {
	tests := []struct {
		name        string
		workerCount int
		shouldFail  bool
	}{
		{"below minimum", 0, true},
		{"minimum valid", 1, false},
		{"typical value", 10, false},
		{"maximum valid", 1000, false},
		{"above maximum", 1001, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				Global: GlobalConfig{
					ScrapeBind:      "0.0.0.0:8080",
					LogLevel:        "info",
					DefaultTimeout:  "5s",
					DefaultInterval: "30s",
					WorkerCount:     tc.workerCount,
					Jitter:          "0s",
				},
			}

			err := cfg.Validate()
			if tc.shouldFail && err == nil {
				t.Error("expected validation error")
			}
			if !tc.shouldFail && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateService_HTTPService(t *testing.T) {
	tests := []struct {
		name       string
		service    Service
		shouldFail bool
		errContain string
	}{
		{
			name: "valid http service",
			service: Service{
				ID:       "web-1",
				Name:     "Web Server",
				Type:     "http",
				URL:      "https://example.com",
				Interval: "30s",
				Timeout:  "5s",
			},
			shouldFail: false,
		},
		{
			name: "missing url",
			service: Service{
				ID:       "web-1",
				Name:     "Web Server",
				Type:     "http",
				URL:      "",
				Interval: "30s",
				Timeout:  "5s",
			},
			shouldFail: true,
			errContain: "url",
		},
		{
			name: "missing id",
			service: Service{
				ID:       "",
				Name:     "Web Server",
				Type:     "http",
				URL:      "https://example.com",
				Interval: "30s",
				Timeout:  "5s",
			},
			shouldFail: true,
			errContain: "id",
		},
		{
			name: "missing name",
			service: Service{
				ID:       "web-1",
				Name:     "",
				Type:     "http",
				URL:      "https://example.com",
				Interval: "30s",
				Timeout:  "5s",
			},
			shouldFail: true,
			errContain: "name",
		},
		{
			name: "invalid id characters",
			service: Service{
				ID:       "web@1!",
				Name:     "Web Server",
				Type:     "http",
				URL:      "https://example.com",
				Interval: "30s",
				Timeout:  "5s",
			},
			shouldFail: true,
			errContain: "invalid characters",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				Global: GlobalConfig{
					ScrapeBind:      "0.0.0.0:8080",
					LogLevel:        "info",
					DefaultTimeout:  "5s",
					DefaultInterval: "30s",
					WorkerCount:     10,
					Jitter:          "0s",
				},
				Services: []Service{tc.service},
			}

			err := cfg.Validate()
			if tc.shouldFail && err == nil {
				t.Error("expected validation error")
			}
			if !tc.shouldFail && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tc.shouldFail && err != nil && tc.errContain != "" {
				if !strings.Contains(strings.ToLower(err.Error()), tc.errContain) {
					t.Errorf("error should contain %q: %v", tc.errContain, err)
				}
			}
		})
	}
}

func TestValidateService_TCPService(t *testing.T) {
	tests := []struct {
		name       string
		service    Service
		shouldFail bool
		errContain string
	}{
		{
			name: "valid tcp service",
			service: Service{
				ID:       "db-1",
				Name:     "Database",
				Type:     "tcp",
				Host:     "localhost",
				Port:     5432,
				Interval: "30s",
				Timeout:  "5s",
			},
			shouldFail: false,
		},
		{
			name: "missing host",
			service: Service{
				ID:       "db-1",
				Name:     "Database",
				Type:     "tcp",
				Host:     "",
				Port:     5432,
				Interval: "30s",
				Timeout:  "5s",
			},
			shouldFail: true,
			errContain: "host",
		},
		{
			name: "port too low",
			service: Service{
				ID:       "db-1",
				Name:     "Database",
				Type:     "tcp",
				Host:     "localhost",
				Port:     0,
				Interval: "30s",
				Timeout:  "5s",
			},
			shouldFail: true,
			errContain: "port",
		},
		{
			name: "port too high",
			service: Service{
				ID:       "db-1",
				Name:     "Database",
				Type:     "tcp",
				Host:     "localhost",
				Port:     65536,
				Interval: "30s",
				Timeout:  "5s",
			},
			shouldFail: true,
			errContain: "port",
		},
		{
			name: "max valid port",
			service: Service{
				ID:       "db-1",
				Name:     "Database",
				Type:     "tcp",
				Host:     "localhost",
				Port:     65535,
				Interval: "30s",
				Timeout:  "5s",
			},
			shouldFail: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				Global: GlobalConfig{
					ScrapeBind:      "0.0.0.0:8080",
					LogLevel:        "info",
					DefaultTimeout:  "5s",
					DefaultInterval: "30s",
					WorkerCount:     10,
					Jitter:          "0s",
				},
				Services: []Service{tc.service},
			}

			err := cfg.Validate()
			if tc.shouldFail && err == nil {
				t.Error("expected validation error")
			}
			if !tc.shouldFail && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tc.shouldFail && err != nil && tc.errContain != "" {
				if !strings.Contains(strings.ToLower(err.Error()), tc.errContain) {
					t.Errorf("error should contain %q: %v", tc.errContain, err)
				}
			}
		})
	}
}

func TestValidateService_DuplicateIDs(t *testing.T) {
	cfg := &Config{
		Global: GlobalConfig{
			ScrapeBind:      "0.0.0.0:8080",
			LogLevel:        "info",
			DefaultTimeout:  "5s",
			DefaultInterval: "30s",
			WorkerCount:     10,
			Jitter:          "0s",
		},
		Services: []Service{
			{ID: "svc-1", Name: "Service 1", Type: "http", URL: "https://example.com", Interval: "30s", Timeout: "5s"},
			{ID: "svc-1", Name: "Service 2", Type: "http", URL: "https://example.org", Interval: "30s", Timeout: "5s"},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("expected validation error for duplicate IDs")
	}
	if !strings.Contains(err.Error(), "duplicated") {
		t.Errorf("error should mention duplicate: %v", err)
	}
}

func TestValidateService_InvalidType(t *testing.T) {
	cfg := &Config{
		Global: GlobalConfig{
			ScrapeBind:      "0.0.0.0:8080",
			LogLevel:        "info",
			DefaultTimeout:  "5s",
			DefaultInterval: "30s",
			WorkerCount:     10,
			Jitter:          "0s",
		},
		Services: []Service{
			{ID: "svc-1", Name: "Service 1", Type: "grpc", URL: "https://example.com", Interval: "30s", Timeout: "5s"},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("expected validation error for invalid type")
	}
	if !strings.Contains(err.Error(), "type") {
		t.Errorf("error should mention type: %v", err)
	}
}

func TestValidateAlerting_ValidChannels(t *testing.T) {
	cfg := &Config{
		Global: GlobalConfig{
			ScrapeBind:      "0.0.0.0:8080",
			LogLevel:        "info",
			DefaultTimeout:  "5s",
			DefaultInterval: "30s",
			WorkerCount:     10,
			Jitter:          "0s",
		},
		Alerting: AlertingConfig{
			Channels: map[string]Channel{
				"discord": {Type: "discord", WebhookURL: "https://discord.com/api/webhooks/123"},
				"slack":   {Type: "slack", WebhookURL: "https://hooks.slack.com/services/xxx"},
				"email": {
					Type:     "email",
					SMTPHost: "smtp.example.com",
					SMTPPort: 587,
					Username: "user",
					Password: "pass",
					From:     "alerts@example.com",
					To:       []string{"admin@example.com"},
				},
			},
		},
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("expected valid config, got error: %v", err)
	}
}

func TestValidateAlerting_InvalidChannels(t *testing.T) {
	tests := []struct {
		name       string
		channel    Channel
		errContain string
	}{
		{
			name:       "discord missing webhook",
			channel:    Channel{Type: "discord", WebhookURL: ""},
			errContain: "webhook_url",
		},
		{
			name:       "slack missing webhook",
			channel:    Channel{Type: "slack", WebhookURL: "   "},
			errContain: "webhook_url",
		},
		{
			name: "email missing smtp_host",
			channel: Channel{
				Type:     "email",
				SMTPHost: "",
				SMTPPort: 587,
				From:     "test@example.com",
				To:       []string{"admin@example.com"},
			},
			errContain: "smtp_host",
		},
		{
			name: "email missing smtp_port",
			channel: Channel{
				Type:     "email",
				SMTPHost: "smtp.example.com",
				SMTPPort: 0,
				From:     "test@example.com",
				To:       []string{"admin@example.com"},
			},
			errContain: "smtp_port",
		},
		{
			name: "email missing from",
			channel: Channel{
				Type:     "email",
				SMTPHost: "smtp.example.com",
				SMTPPort: 587,
				From:     "",
				To:       []string{"admin@example.com"},
			},
			errContain: "from",
		},
		{
			name: "email missing to",
			channel: Channel{
				Type:     "email",
				SMTPHost: "smtp.example.com",
				SMTPPort: 587,
				From:     "test@example.com",
				To:       []string{},
			},
			errContain: "to",
		},
		{
			name: "email username without password",
			channel: Channel{
				Type:     "email",
				SMTPHost: "smtp.example.com",
				SMTPPort: 587,
				Username: "user",
				Password: "",
				From:     "test@example.com",
				To:       []string{"admin@example.com"},
			},
			errContain: "username and password",
		},
		{
			name:       "invalid channel type",
			channel:    Channel{Type: "telegram"},
			errContain: "type",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				Global: GlobalConfig{
					ScrapeBind:      "0.0.0.0:8080",
					LogLevel:        "info",
					DefaultTimeout:  "5s",
					DefaultInterval: "30s",
					WorkerCount:     10,
					Jitter:          "0s",
				},
				Alerting: AlertingConfig{
					Channels: map[string]Channel{"test": tc.channel},
				},
			}

			err := cfg.Validate()
			if err == nil {
				t.Error("expected validation error")
			}
			if !strings.Contains(strings.ToLower(err.Error()), tc.errContain) {
				t.Errorf("error should contain %q: %v", tc.errContain, err)
			}
		})
	}
}

func TestValidateAlerting_RouteReferencesUndefinedChannel(t *testing.T) {
	cfg := &Config{
		Global: GlobalConfig{
			ScrapeBind:      "0.0.0.0:8080",
			LogLevel:        "info",
			DefaultTimeout:  "5s",
			DefaultInterval: "30s",
			WorkerCount:     10,
			Jitter:          "0s",
		},
		Alerting: AlertingConfig{
			Channels: map[string]Channel{
				"discord": {Type: "discord", WebhookURL: "https://discord.com/api/webhooks/123"},
			},
			Routes: []Route{
				{
					Match:  RouteMatch{ServiceIDs: []string{"svc-1"}},
					Notify: []string{"nonexistent-channel"},
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("expected validation error for undefined channel reference")
	}
	if !strings.Contains(err.Error(), "undefined channel") {
		t.Errorf("error should mention undefined channel: %v", err)
	}
}

func TestValidateAlerting_InvalidCooldown(t *testing.T) {
	cfg := &Config{
		Global: GlobalConfig{
			ScrapeBind:      "0.0.0.0:8080",
			LogLevel:        "info",
			DefaultTimeout:  "5s",
			DefaultInterval: "30s",
			WorkerCount:     10,
			Jitter:          "0s",
		},
		Alerting: AlertingConfig{
			Channels: map[string]Channel{
				"discord": {Type: "discord", WebhookURL: "https://discord.com/api/webhooks/123"},
			},
			Routes: []Route{
				{
					Match:  RouteMatch{ServiceIDs: []string{"svc-1"}},
					Policy: RoutePolicy{Cooldown: "invalid-duration"},
					Notify: []string{"discord"},
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("expected validation error for invalid cooldown")
	}
	if !strings.Contains(err.Error(), "cooldown") {
		t.Errorf("error should mention cooldown: %v", err)
	}
}

func TestValidateAlerting_RoutesWithoutChannels(t *testing.T) {
	cfg := &Config{
		Global: GlobalConfig{
			ScrapeBind:      "0.0.0.0:8080",
			LogLevel:        "info",
			DefaultTimeout:  "5s",
			DefaultInterval: "30s",
			WorkerCount:     10,
			Jitter:          "0s",
		},
		Alerting: AlertingConfig{
			Channels: map[string]Channel{},
			Routes: []Route{
				{
					Match:  RouteMatch{ServiceIDs: []string{"svc-1"}},
					Notify: []string{"discord"},
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("expected validation error for routes without channels")
	}
}

func TestValidationError_Format(t *testing.T) {
	ve := ValidationError{
		Errors: []string{"error one", "error two", "error three"},
	}

	errStr := ve.Error()
	if !strings.Contains(errStr, "error one") {
		t.Error("error string should contain 'error one'")
	}
	if !strings.Contains(errStr, "error two") {
		t.Error("error string should contain 'error two'")
	}
	if !strings.Contains(errStr, "validation failed") {
		t.Error("error string should contain 'validation failed'")
	}
}

func TestIsSafeID(t *testing.T) {
	tests := []struct {
		id     string
		expect bool
	}{
		{"valid-id", true},
		{"valid_id", true},
		{"ValidID123", true},
		{"123", true},
		{"a", true},
		{"", false},
		{"invalid@id", false},
		{"invalid id", false},
		{"invalid.id", false},
		{"invalid/id", false},
	}

	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			got := isSafeID(tc.id)
			if got != tc.expect {
				t.Errorf("isSafeID(%q) = %v, want %v", tc.id, got, tc.expect)
			}
		})
	}
}
