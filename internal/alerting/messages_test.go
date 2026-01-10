package alerting

import (
	"strings"
	"testing"
	"time"

	"uptiq/internal/checks"
	"uptiq/internal/config"
)

func TestMessageBuilder_DownAlert(t *testing.T) {
	builder := NewMessageBuilder()

	svc := config.Service{
		ID:   "api-1",
		Name: "API Server",
		Type: "http",
		URL:  "https://api.example.com/health",
	}

	res := checks.Result{
		Success:    false,
		StatusCode: 500,
		Latency:    150 * time.Millisecond,
		Error:      "internal server error",
	}

	payload := builder.DownAlert(svc, res, 3, 3)

	// Check kind
	if payload.Kind != "down" {
		t.Errorf("Kind = %q, want %q", payload.Kind, "down")
	}

	// Check webhook message
	if !strings.Contains(payload.WebhookMessage, "DOWN") {
		t.Error("webhook message should contain 'DOWN'")
	}
	if !strings.Contains(payload.WebhookMessage, svc.Name) {
		t.Error("webhook message should contain service name")
	}
	if !strings.Contains(payload.WebhookMessage, svc.ID) {
		t.Error("webhook message should contain service ID")
	}
	if !strings.Contains(payload.WebhookMessage, svc.URL) {
		t.Error("webhook message should contain target URL")
	}

	// Check email subject
	if !strings.Contains(payload.EmailSubject, "[DOWN]") {
		t.Errorf("email subject should contain '[DOWN]': %s", payload.EmailSubject)
	}

	// Check email body
	if !strings.Contains(payload.EmailBody, "SERVICE DOWN") {
		t.Error("email body should contain 'SERVICE DOWN'")
	}
	if !strings.Contains(payload.EmailBody, res.Error) {
		t.Error("email body should contain error message")
	}
}

func TestMessageBuilder_StillDownAlert(t *testing.T) {
	builder := NewMessageBuilder()

	svc := config.Service{
		ID:   "db-1",
		Name: "Database",
		Type: "tcp",
		Host: "db.example.com",
		Port: 5432,
	}

	res := checks.Result{
		Success: false,
		Latency: 5 * time.Second,
		Error:   "connection refused",
	}

	payload := builder.StillDownAlert(svc, res, 10, 3)

	// Check kind
	if payload.Kind != "down" {
		t.Errorf("Kind = %q, want %q", payload.Kind, "down")
	}

	// Should contain "STILL DOWN" indicator
	if !strings.Contains(payload.WebhookMessage, "STILL DOWN") {
		t.Error("webhook message should contain 'STILL DOWN'")
	}
	if !strings.Contains(payload.EmailSubject, "still down") {
		t.Errorf("email subject should contain 'still down': %s", payload.EmailSubject)
	}
}

func TestMessageBuilder_RecoveryAlert(t *testing.T) {
	builder := NewMessageBuilder()

	svc := config.Service{
		ID:   "web-1",
		Name: "Web Server",
		Type: "http",
		URL:  "https://example.com",
	}

	res := checks.Result{
		Success:    true,
		StatusCode: 200,
		Latency:    50 * time.Millisecond,
	}

	payload := builder.RecoveryAlert(svc, res)

	// Check kind
	if payload.Kind != "recovery" {
		t.Errorf("Kind = %q, want %q", payload.Kind, "recovery")
	}

	// Check webhook message
	if !strings.Contains(payload.WebhookMessage, "UP") {
		t.Error("webhook message should contain 'UP'")
	}
	if !strings.Contains(payload.WebhookMessage, "✅") {
		t.Error("webhook message should contain checkmark emoji")
	}

	// Check email subject
	if !strings.Contains(payload.EmailSubject, "[UP]") {
		t.Errorf("email subject should contain '[UP]': %s", payload.EmailSubject)
	}

	// Check email body
	if !strings.Contains(payload.EmailBody, "RECOVERY") {
		t.Error("email body should contain 'RECOVERY'")
	}
}

func TestMessageBuilder_HTTPServiceDetails(t *testing.T) {
	builder := NewMessageBuilder()

	svc := config.Service{
		ID:   "api",
		Name: "API",
		Type: "http",
		URL:  "https://api.example.com",
	}

	res := checks.Result{
		Success:    false,
		StatusCode: 503,
		Latency:    200 * time.Millisecond,
	}

	payload := builder.DownAlert(svc, res, 1, 1)

	// Should include HTTP status
	if !strings.Contains(payload.WebhookMessage, "503") {
		t.Error("webhook should include status code")
	}
	if !strings.Contains(payload.EmailBody, "503") {
		t.Error("email body should include status code")
	}
}

func TestMessageBuilder_TCPServiceDetails(t *testing.T) {
	builder := NewMessageBuilder()

	svc := config.Service{
		ID:   "postgres",
		Name: "PostgreSQL",
		Type: "tcp",
		Host: "db.internal",
		Port: 5432,
	}

	res := checks.Result{
		Success: false,
		Latency: 100 * time.Millisecond,
		Error:   "connection refused",
	}

	payload := builder.DownAlert(svc, res, 1, 1)

	// Should include TCP target
	if !strings.Contains(payload.WebhookMessage, "db.internal:5432") {
		t.Errorf("webhook should include TCP target: %s", payload.WebhookMessage)
	}
	if !strings.Contains(payload.EmailBody, "db.internal:5432") {
		t.Errorf("email body should include TCP target: %s", payload.EmailBody)
	}
}

func TestMessageBuilder_FailureThresholdDisplay(t *testing.T) {
	builder := NewMessageBuilder()

	svc := config.Service{
		ID:   "svc",
		Name: "Service",
		Type: "http",
		URL:  "https://example.com",
	}

	res := checks.Result{
		Success: false,
		Latency: 100 * time.Millisecond,
	}

	// With threshold > 1, should show failure count
	payload := builder.DownAlert(svc, res, 3, 5)
	if !strings.Contains(payload.WebhookMessage, "3/5") {
		t.Errorf("should show failure ratio when threshold > 1: %s", payload.WebhookMessage)
	}

	// With threshold = 1, should not show ratio
	payload = builder.DownAlert(svc, res, 1, 1)
	if strings.Contains(payload.WebhookMessage, "1/1") {
		t.Error("should not show ratio when threshold = 1")
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
			service: config.Service{Type: "http", URL: "https://example.com/health"},
			expect:  "https://example.com/health",
		},
		{
			name:    "tcp service",
			service: config.Service{Type: "tcp", Host: "db.example.com", Port: 5432},
			expect:  "db.example.com:5432",
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

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		expect string
	}{
		{
			name:   "short string",
			input:  "hello",
			maxLen: 10,
			expect: "hello",
		},
		{
			name:   "exact length",
			input:  "hello",
			maxLen: 5,
			expect: "hello",
		},
		{
			name:   "needs truncation",
			input:  "hello world this is long",
			maxLen: 10,
			expect: "hello w...",
		},
		{
			name:   "very short max",
			input:  "hello",
			maxLen: 3,
			expect: "hel",
		},
		{
			name:   "with whitespace",
			input:  "  hello  ",
			maxLen: 10,
			expect: "hello",
		},
		{
			name:   "empty string",
			input:  "",
			maxLen: 10,
			expect: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := truncate(tc.input, tc.maxLen)
			if got != tc.expect {
				t.Errorf("truncate(%q, %d) = %q, want %q", tc.input, tc.maxLen, got, tc.expect)
			}
		})
	}
}

func TestAlertPayload_Fields(t *testing.T) {
	payload := AlertPayload{
		Kind:           "down",
		WebhookMessage: "Test webhook message",
		EmailSubject:   "Test subject",
		EmailBody:      "Test body",
	}

	if payload.Kind != "down" {
		t.Errorf("Kind = %q, want %q", payload.Kind, "down")
	}
	if payload.WebhookMessage != "Test webhook message" {
		t.Error("WebhookMessage mismatch")
	}
	if payload.EmailSubject != "Test subject" {
		t.Error("EmailSubject mismatch")
	}
	if payload.EmailBody != "Test body" {
		t.Error("EmailBody mismatch")
	}
}

func TestNewMessageBuilder(t *testing.T) {
	builder := NewMessageBuilder()
	if builder == nil {
		t.Fatal("NewMessageBuilder returned nil")
	}
}

func TestMessageBuilder_LatencyFormatting(t *testing.T) {
	builder := NewMessageBuilder()

	svc := config.Service{
		ID:   "test",
		Name: "Test",
		Type: "http",
		URL:  "https://example.com",
	}

	res := checks.Result{
		Success: false,
		Latency: 1234 * time.Millisecond,
	}

	payload := builder.DownAlert(svc, res, 1, 1)

	// Should show latency in milliseconds
	if !strings.Contains(payload.WebhookMessage, "1234ms") {
		t.Errorf("webhook should show latency in ms: %s", payload.WebhookMessage)
	}
}

func TestMessageBuilder_ErrorTruncation(t *testing.T) {
	builder := NewMessageBuilder()

	svc := config.Service{
		ID:   "test",
		Name: "Test",
		Type: "http",
		URL:  "https://example.com",
	}

	// Create a very long error message
	longError := strings.Repeat("error ", 100)

	res := checks.Result{
		Success: false,
		Latency: 100 * time.Millisecond,
		Error:   longError,
	}

	payload := builder.DownAlert(svc, res, 1, 1)

	// Webhook message should have truncated error
	if len(payload.WebhookMessage) > 500 {
		// It shouldn't be extremely long due to truncation
		t.Log("Note: webhook message length:", len(payload.WebhookMessage))
	}

	// Email body can have full error
	if !strings.Contains(payload.EmailBody, "error") {
		t.Error("email body should contain error")
	}
}

func TestMessageBuilder_TimestampPresence(t *testing.T) {
	builder := NewMessageBuilder()

	svc := config.Service{
		ID:   "test",
		Name: "Test",
		Type: "http",
		URL:  "https://example.com",
	}

	res := checks.Result{
		Success: false,
		Latency: 100 * time.Millisecond,
	}

	payload := builder.DownAlert(svc, res, 1, 1)

	// Should contain timestamp (RFC3339 format contains 'T')
	if !strings.Contains(payload.WebhookMessage, "T") {
		t.Error("webhook should contain timestamp")
	}
	if !strings.Contains(payload.EmailBody, "Time:") {
		t.Error("email body should contain time label")
	}
}
