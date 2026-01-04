package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sitelert/internal/checks"
	"sitelert/internal/config"
	"slices"
	"strings"
	"sync"
	"time"
)

type AlertState string

const (
	StateUnknown AlertState = "UNKNOWN"
	StateUp      AlertState = "UP"
	StateDown    AlertState = "DOWN"
)

type serviceState struct {
	State               AlertState
	ConsecutiveFailures int

	// For DOWN alert throttling
	LastDownAlertAt time.Time
	DownNotified    bool // whether we sent a DOWN alert for the current/most recent outage episode

	// For recovery behavior
	LastResultAt time.Time
}

type compiledRoute struct {
	matchServiceIDs []string
	notify          []string
	policy          compiledPolicy
}

type compiledPolicy struct {
	failureThreshold int
	cooldown         time.Duration
	recoveryAlert    bool
}

type Engine struct {
	log      *slog.Logger
	client   *http.Client
	channels map[string]config.Channel

	// routing
	routes     []compiledRoute
	routeIndex map[string][]int // service_id -> route indices

	// state
	mu    sync.Mutex
	state map[string]*serviceState
}

func NewEngine(cfg config.AlertingConfig, log *slog.Logger) *Engine {
	if log == nil {
		log = slog.Default()
	}

	e := &Engine{
		log:        log,
		client:     &http.Client{Timeout: 7 * time.Second},
		channels:   cfg.Channels,
		routeIndex: make(map[string][]int),
		state:      make(map[string]*serviceState),
	}

	// Compile routes once
	for i, r := range cfg.Routes {
		cr := compiledRoute{
			matchServiceIDs: trimAll(r.Match.ServiceIDs),
			notify:          trimAll(r.Notify),
			policy:          compilePolicy(r.Policy),
		}
		e.routes = append(e.routes, cr)

		for _, id := range cr.matchServiceIDs {
			if id == "" {
				continue
			}
			e.routeIndex[id] = append(e.routeIndex[id], i)
		}
	}

	return e
}

func compilePolicy(p config.RoutePolicy) compiledPolicy {
	// defaults
	out := compiledPolicy{
		failureThreshold: 1,
		cooldown:         0,
		recoveryAlert:    p.RecoveryAlert,
	}
	if p.FailureThreshold > 0 {
		out.failureThreshold = p.FailureThreshold
	}
	if strings.TrimSpace(p.Cooldown) != "" {
		if d, err := time.ParseDuration(p.Cooldown); err == nil && d > 0 {
			out.cooldown = d
		}
	}
	return out
}

func trimAll(in []string) []string {
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// ---- Routing ----

type resolvedRoute struct {
	notify []string
	policy compiledPolicy
	ok     bool
}

// resolveRoute unions channels across all matching routes and merges policy conservatively:
// - failure_threshold: max (reduces spam)
// - cooldown: max (reduces spam)
// - recovery_alert: true if any route enables it
func (e *Engine) resolveRoute(serviceID string) resolvedRoute {
	idxs := e.routeIndex[serviceID]
	if len(idxs) == 0 {
		return resolvedRoute{ok: false}
	}

	var notify []string
	policy := compiledPolicy{failureThreshold: 1} // baseline

	first := true
	for _, idx := range idxs {
		r := e.routes[idx]
		for _, ch := range r.notify {
			if !slices.Contains(notify, ch) {
				notify = append(notify, ch)
			}
		}
		if first {
			policy = r.policy
			first = false
		} else {
			// merge
			if r.policy.failureThreshold > policy.failureThreshold {
				policy.failureThreshold = r.policy.failureThreshold
			}
			if r.policy.cooldown > policy.cooldown {
				policy.cooldown = r.policy.cooldown
			}
			if r.policy.recoveryAlert {
				policy.recoveryAlert = true
			}
		}
	}

	if len(notify) == 0 {
		return resolvedRoute{ok: false}
	}

	return resolvedRoute{notify: notify, policy: policy, ok: true}
}

// ---- Dispatch payload (payload change for Milestone 6) ----

type dispatchPayload struct {
	kind string // "down" | "recovery"

	webhookMessage string // used for discord/slack

	emailSubject string
	emailBody    string
}

// ---- Public API called by scheduler ----

func (e *Engine) HandleResult(svc config.Service, res checks.Result) {
	route := e.resolveRoute(svc.ID)
	if !route.ok {
		// no routing configured for this service
		return
	}

	now := time.Now()

	var (
		sendDown      bool
		sendDownAgain bool
		sendRecovery  bool

		payload dispatchPayload
	)

	e.mu.Lock()
	st := e.state[svc.ID]
	if st == nil {
		st = &serviceState{State: StateUnknown}
		e.state[svc.ID] = st
	}
	st.LastResultAt = now

	if res.Success {
		// Reset failure streak
		st.ConsecutiveFailures = 0

		// Recovery alert if we were DOWN and had previously sent a down alert
		if st.State == StateDown {
			if route.policy.recoveryAlert && st.DownNotified {
				sendRecovery = true
				payload = dispatchPayload{
					kind:           "recovery",
					webhookMessage: formatRecoveryMessage(svc, res),
					emailSubject:   emailSubjectRecovery(svc),
					emailBody:      emailBodyRecovery(svc, res),
				}
			}
			// new episode begins; reset flag
			st.DownNotified = false
		}
		st.State = StateUp
		e.mu.Unlock()

		if sendRecovery {
			e.dispatch(route.notify, svc, payload)
		}
		return
	}

	// failure case
	st.ConsecutiveFailures++

	// Not yet considered "down" until threshold reached
	if st.ConsecutiveFailures < route.policy.failureThreshold {
		// Keep state UP unless already DOWN
		if st.State == StateUnknown {
			st.State = StateUp
		}
		e.mu.Unlock()
		return
	}

	// Threshold reached: service is DOWN
	wasDown := st.State == StateDown
	st.State = StateDown

	// Decide if we can send a DOWN alert now (cooldown)
	canSendDown := route.policy.cooldown <= 0 ||
		st.LastDownAlertAt.IsZero() ||
		now.Sub(st.LastDownAlertAt) >= route.policy.cooldown

	// First DOWN alert of an outage episode
	if !st.DownNotified && canSendDown {
		sendDown = true
		st.DownNotified = true
		st.LastDownAlertAt = now

		payload = dispatchPayload{
			kind:           "down",
			webhookMessage: formatDownMessage(svc, res, st.ConsecutiveFailures, route.policy.failureThreshold, false),
			emailSubject:   emailSubjectDown(svc),
			emailBody:      emailBodyDown(svc, res, st.ConsecutiveFailures, route.policy.failureThreshold),
		}
	} else if wasDown && st.DownNotified && canSendDown {
		// Optional reminder while still down (cooldown elapsed)
		sendDownAgain = true
		st.LastDownAlertAt = now

		subj := emailSubjectDown(svc) + " (still down)"
		payload = dispatchPayload{
			kind:           "down",
			webhookMessage: formatDownMessage(svc, res, st.ConsecutiveFailures, route.policy.failureThreshold, true),
			emailSubject:   subj,
			emailBody:      emailBodyDown(svc, res, st.ConsecutiveFailures, route.policy.failureThreshold),
		}
	}
	e.mu.Unlock()

	if sendDown || sendDownAgain {
		e.dispatch(route.notify, svc, payload)
	}
}

// ---- Dispatch to channels ----

func (e *Engine) dispatch(channelNames []string, svc config.Service, p dispatchPayload) {
	for _, name := range channelNames {
		ch, ok := e.channels[name]
		if !ok {
			e.log.Warn("alert channel missing",
				"channel", name,
				"service_id", svc.ID,
				"service_name", svc.Name,
			)
			continue
		}

		switch strings.ToLower(strings.TrimSpace(ch.Type)) {
		case "discord":
			if err := e.sendDiscord(ch.WebhookURL, p.webhookMessage); err != nil {
				e.log.Warn("discord send failed",
					"channel", name,
					"service_id", svc.ID,
					"service_name", svc.Name,
					"kind", p.kind,
					"error", err.Error(),
				)
			} else {
				e.log.Info("discord alert sent",
					"channel", name,
					"service_id", svc.ID,
					"service_name", svc.Name,
					"kind", p.kind,
				)
			}

		case "slack":
			if err := e.sendSlack(ch.WebhookURL, p.webhookMessage); err != nil {
				e.log.Warn("slack send failed",
					"channel", name,
					"service_id", svc.ID,
					"service_name", svc.Name,
					"kind", p.kind,
					"error", err.Error(),
				)
			} else {
				e.log.Info("slack alert sent",
					"channel", name,
					"service_id", svc.ID,
					"service_name", svc.Name,
					"kind", p.kind,
				)
			}

		case "email":
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			// Redaction: do NOT log username/password.
			authConfigured := strings.TrimSpace(ch.Username) != "" || strings.TrimSpace(ch.Password) != ""

			subj := p.emailSubject
			body := p.emailBody
			if strings.TrimSpace(subj) == "" {
				subj = fmt.Sprintf("[ALERT] %s (%s)", svc.Name, svc.ID)
			}
			if strings.TrimSpace(body) == "" {
				body = p.webhookMessage
			}

			if err := e.sendEmail(ctx, ch, subj, body); err != nil {
				e.log.Warn("email send failed",
					"channel", name,
					"smtp_host", ch.SMTPHost,
					"smtp_port", ch.SMTPPort,
					"auth", authConfigured,
					"to_count", len(ch.To),
					"service_id", svc.ID,
					"service_name", svc.Name,
					"kind", p.kind,
					"error", err.Error(),
				)
			} else {
				e.log.Info("email alert sent",
					"channel", name,
					"smtp_host", ch.SMTPHost,
					"smtp_port", ch.SMTPPort,
					"auth", authConfigured,
					"to_count", len(ch.To),
					"service_id", svc.ID,
					"service_name", svc.Name,
					"kind", p.kind,
				)
			}

		default:
			e.log.Warn("unsupported channel type (milestone 6 supports discord/slack/email)",
				"channel", name,
				"type", ch.Type,
				"service_id", svc.ID,
				"service_name", svc.Name,
				"kind", p.kind,
			)
		}
	}
}

func (e *Engine) sendDiscord(webhookURL, msg string) error {
	if strings.TrimSpace(webhookURL) == "" {
		return errors.New("empty discord webhook_url")
	}
	payload := map[string]string{"content": msg}
	return e.postJSON(webhookURL, payload)
}

func (e *Engine) sendSlack(webhookURL, msg string) error {
	if strings.TrimSpace(webhookURL) == "" {
		return errors.New("empty slack webhook_url")
	}
	payload := map[string]string{"text": msg}
	return e.postJSON(webhookURL, payload)
}

func (e *Engine) postJSON(url string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("non-2xx status: %s", resp.Status)
	}
	return nil
}
