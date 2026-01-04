package alerting

import (
	"fmt"
	"sitelert/internal/checks"
	"sitelert/internal/config"
	"strings"
	"time"
)

func emailSubjectDown(svc config.Service) string {
	return fmt.Sprintf("[DOWN] %s (%s)", svc.Name, svc.ID)
}

func emailSubjectRecovery(svc config.Service) string {
	return fmt.Sprintf("[UP] %s (%s)", svc.Name, svc.ID)
}

func emailBodyDown(svc config.Service, res checks.Result, failures, threshold int) string {
	var sb strings.Builder
	sb.WriteString("ALERT: SERVICE DOWN\n")
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("Service: %s\n", svc.Name))
	sb.WriteString(fmt.Sprintf("ID: %s\n", svc.ID))
	sb.WriteString(fmt.Sprintf("Type: %s\n", strings.ToLower(svc.Type)))
	sb.WriteString(fmt.Sprintf("Target: %s\n", targetForService(svc)))
	if res.StatusCode != 0 {
		sb.WriteString(fmt.Sprintf("HTTP Status: %d\n", res.StatusCode))
	}
	sb.WriteString(fmt.Sprintf("Latency: %dms\n", res.Latency.Milliseconds()))
	if threshold > 1 {
		sb.WriteString(fmt.Sprintf("Consecutive failures: %d/%d\n", failures, threshold))
	}
	if strings.TrimSpace(res.Error) != "" {
		sb.WriteString(fmt.Sprintf("Error: %s\n", res.Error))
	}
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("Time: %s\n", time.Now().Format(time.RFC3339)))
	sb.WriteString("\n")
	sb.WriteString("Next steps:\n")
	sb.WriteString("- Check service health and recent deploys\n")
	sb.WriteString("- Verify DNS/network reachability\n")
	sb.WriteString("- Review logs / monitoring dashboards\n")
	return sb.String()
}

func emailBodyRecovery(svc config.Service, res checks.Result) string {
	var sb strings.Builder
	sb.WriteString("RECOVERY: SERVICE UP\n")
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("Service: %s\n", svc.Name))
	sb.WriteString(fmt.Sprintf("ID: %s\n", svc.ID))
	sb.WriteString(fmt.Sprintf("Type: %s\n", strings.ToLower(svc.Type)))
	sb.WriteString(fmt.Sprintf("Target: %s\n", targetForService(svc)))
	if res.StatusCode != 0 {
		sb.WriteString(fmt.Sprintf("HTTP Status: %d\n", res.StatusCode))
	}
	sb.WriteString(fmt.Sprintf("Latency: %dms\n", res.Latency.Milliseconds()))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("Time: %s\n", time.Now().Format(time.RFC3339)))
	return sb.String()
}
