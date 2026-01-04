package alerting

import (
	"fmt"
	"net"
	"sitelert/internal/checks"
	"sitelert/internal/config"
	"strings"
	"time"
)

func targetForService(svc config.Service) string {
	if strings.EqualFold(svc.Type, "http") {
		return svc.URL
	}
	if strings.EqualFold(svc.Type, "tcp") {
		return net.JoinHostPort(svc.Host, fmt.Sprintf("%d", svc.Port))
	}

	return ""
}

func formatDownMessage(svc config.Service, res checks.Result, failures, threshold int, reminder bool) string {
	target := targetForService(svc)
	lat := fmt.Sprintf("%.0fms", float64(res.Latency.Milliseconds()))
	ts := time.Now().Format(time.RFC3339)

	prefix := "🚨 DOWN"
	if reminder {
		prefix = "🚨 STILL DOWN"
	}

	statusPart := ""
	if res.StatusCode != 0 {
		statusPart = fmt.Sprintf(" status=%d", res.StatusCode)
	}

	errPart := ""
	if strings.TrimSpace(res.Error) != "" {
		errPart = fmt.Sprintf(" err=%q", truncate(res.Error, 180))
	}

	thrPart := ""
	if threshold > 1 {
		thrPart = fmt.Sprintf(" (failure=%d/%d)", failures, threshold)
	}

	return fmt.Sprintf("%s: %s (%s) [%s]%s\ntarget=%s%s latency=%s at=%s", prefix, svc.Name, svc.ID, strings.ToLower(svc.Type), thrPart, target, statusPart, lat, ts) + errPart
}

func formatRecoveryMessage(svc config.Service, res checks.Result) string {
	target := targetForService(svc)
	lat := fmt.Sprintf("%.0fms", float64(res.Latency.Milliseconds()))
	ts := time.Now().Format(time.RFC3339)

	statusPart := ""
	if res.StatusCode != 0 {
		statusPart = fmt.Sprintf(" status=%d", res.StatusCode)
	}

	return fmt.Sprintf("✅ UP: %s (%s) [%s]\ntarget=%s%s latency=%s at=%s", svc.Name, svc.ID, strings.ToLower(svc.Type), target, statusPart, lat, ts)
}

func truncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
