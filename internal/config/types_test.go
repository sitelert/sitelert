package config

import "testing"

func TestService_IsHTTP(t *testing.T) {
	tests := []struct {
		serviceType string
		expected    bool
	}{
		{"http", true},
		{"HTTP", false}, // case sensitive
		{"tcp", false},
		{"", false},
		{"grpc", false},
	}

	for _, tc := range tests {
		t.Run(tc.serviceType, func(t *testing.T) {
			svc := Service{Type: tc.serviceType}
			if got := svc.IsHTTP(); got != tc.expected {
				t.Errorf("IsHTTP() for type %q = %v, want %v", tc.serviceType, got, tc.expected)
			}
		})
	}
}

func TestService_IsTCP(t *testing.T) {
	tests := []struct {
		serviceType string
		expected    bool
	}{
		{"tcp", true},
		{"TCP", false}, // case sensitive
		{"http", false},
		{"", false},
		{"udp", false},
	}

	for _, tc := range tests {
		t.Run(tc.serviceType, func(t *testing.T) {
			svc := Service{Type: tc.serviceType}
			if got := svc.IsTCP(); got != tc.expected {
				t.Errorf("IsTCP() for type %q = %v, want %v", tc.serviceType, got, tc.expected)
			}
		})
	}
}

func TestServiceType_Constants(t *testing.T) {
	if ServiceTypeHTTP != "http" {
		t.Errorf("ServiceTypeHTTP = %q, want %q", ServiceTypeHTTP, "http")
	}
	if ServiceTypeTCP != "tcp" {
		t.Errorf("ServiceTypeTCP = %q, want %q", ServiceTypeTCP, "tcp")
	}
}

func TestChannelType_Constants(t *testing.T) {
	if ChannelTypeDiscord != "discord" {
		t.Errorf("ChannelTypeDiscord = %q, want %q", ChannelTypeDiscord, "discord")
	}
	if ChannelTypeSlack != "slack" {
		t.Errorf("ChannelTypeSlack = %q, want %q", ChannelTypeSlack, "slack")
	}
	if ChannelTypeEmail != "email" {
		t.Errorf("ChannelTypeEmail = %q, want %q", ChannelTypeEmail, "email")
	}
}

func TestConfig_StructFields(t *testing.T) {
	// Ensure Config struct can be instantiated with all fields
	cfg := Config{
		Global: GlobalConfig{
			ScrapeBind:      "0.0.0.0:8080",
			LogLevel:        "info",
			DefaultTimeout:  "5s",
			DefaultInterval: "30s",
			WorkerCount:     10,
			Jitter:          "500ms",
		},
		Services: []Service{
			{
				ID:             "web",
				Name:           "Web Server",
				Type:           "http",
				URL:            "https://example.com",
				Method:         "GET",
				ExpectedStatus: []int{200, 201},
				Contains:       "OK",
				Headers:        map[string]string{"Authorization": "Bearer token"},
				Interval:       "30s",
				Timeout:        "5s",
			},
			{
				ID:       "db",
				Name:     "Database",
				Type:     "tcp",
				Host:     "localhost",
				Port:     5432,
				Interval: "15s",
				Timeout:  "3s",
			},
		},
		Alerting: AlertingConfig{
			Channels: map[string]Channel{
				"discord": {
					Type:       "discord",
					WebhookURL: "https://discord.com/webhooks/123",
				},
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
			Routes: []Route{
				{
					Match: RouteMatch{
						ServiceIDs: []string{"web", "db"},
					},
					Policy: RoutePolicy{
						FailureThreshold: 3,
						Cooldown:         "5m",
						RecoveryAlert:    true,
					},
					Notify: []string{"discord", "email"},
				},
			},
		},
	}

	// Basic validation that struct was populated correctly
	if len(cfg.Services) != 2 {
		t.Errorf("expected 2 services, got %d", len(cfg.Services))
	}
	if len(cfg.Alerting.Channels) != 2 {
		t.Errorf("expected 2 channels, got %d", len(cfg.Alerting.Channels))
	}
	if len(cfg.Alerting.Routes) != 1 {
		t.Errorf("expected 1 route, got %d", len(cfg.Alerting.Routes))
	}
}

func TestService_HTTPFields(t *testing.T) {
	svc := Service{
		ID:             "api",
		Name:           "API Server",
		Type:           "http",
		URL:            "https://api.example.com/health",
		Method:         "POST",
		ExpectedStatus: []int{200, 202, 204},
		Contains:       `"status":"ok"`,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer test",
		},
		Interval: "15s",
		Timeout:  "3s",
	}

	if !svc.IsHTTP() {
		t.Error("expected IsHTTP() to return true")
	}
	if svc.IsTCP() {
		t.Error("expected IsTCP() to return false")
	}
	if len(svc.ExpectedStatus) != 3 {
		t.Errorf("expected 3 status codes, got %d", len(svc.ExpectedStatus))
	}
	if len(svc.Headers) != 2 {
		t.Errorf("expected 2 headers, got %d", len(svc.Headers))
	}
}

func TestService_TCPFields(t *testing.T) {
	svc := Service{
		ID:       "postgres",
		Name:     "PostgreSQL",
		Type:     "tcp",
		Host:     "db.example.com",
		Port:     5432,
		Interval: "10s",
		Timeout:  "2s",
	}

	if svc.IsHTTP() {
		t.Error("expected IsHTTP() to return false")
	}
	if !svc.IsTCP() {
		t.Error("expected IsTCP() to return true")
	}
	if svc.Host != "db.example.com" {
		t.Errorf("Host = %q, want %q", svc.Host, "db.example.com")
	}
	if svc.Port != 5432 {
		t.Errorf("Port = %d, want %d", svc.Port, 5432)
	}
}

func TestChannel_AllTypes(t *testing.T) {
	// Discord channel
	discord := Channel{
		Type:       "discord",
		WebhookURL: "https://discord.com/api/webhooks/123/abc",
	}
	if discord.Type != "discord" {
		t.Errorf("discord Type = %q, want %q", discord.Type, "discord")
	}

	// Slack channel
	slack := Channel{
		Type:       "slack",
		WebhookURL: "https://hooks.slack.com/services/T/B/X",
	}
	if slack.Type != "slack" {
		t.Errorf("slack Type = %q, want %q", slack.Type, "slack")
	}

	// Email channel
	email := Channel{
		Type:     "email",
		SMTPHost: "smtp.gmail.com",
		SMTPPort: 587,
		Username: "user@gmail.com",
		Password: "app-password",
		From:     "Alerts <alerts@example.com>",
		To:       []string{"team@example.com", "oncall@example.com"},
	}
	if email.Type != "email" {
		t.Errorf("email Type = %q, want %q", email.Type, "email")
	}
	if len(email.To) != 2 {
		t.Errorf("email To recipients = %d, want 2", len(email.To))
	}
}

func TestRoutePolicy_Defaults(t *testing.T) {
	// Empty policy should have zero values
	policy := RoutePolicy{}

	if policy.FailureThreshold != 0 {
		t.Errorf("default FailureThreshold = %d, want 0", policy.FailureThreshold)
	}
	if policy.Cooldown != "" {
		t.Errorf("default Cooldown = %q, want empty", policy.Cooldown)
	}
	if policy.RecoveryAlert != false {
		t.Error("default RecoveryAlert should be false")
	}
}

func TestRouteMatch_ServiceIDs(t *testing.T) {
	match := RouteMatch{
		ServiceIDs: []string{"svc-1", "svc-2", "svc-3"},
	}

	if len(match.ServiceIDs) != 3 {
		t.Errorf("expected 3 service IDs, got %d", len(match.ServiceIDs))
	}
}
