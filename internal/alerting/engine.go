package alerting

import (
	"log/slog"
	"sitelert/internal/checks"
	"sitelert/internal/config"
	"slices"
	"strings"
)

type Engine struct {
	log      *slog.Logger
	channels map[string]config.Channel
	routes   []config.Route

	routeIndex map[string][]int
}

func NewEngine(cfg config.AlertingConfig, log *slog.Logger) *Engine {
	if log == nil {
		log = slog.Default()
	}

	e := &Engine{
		log:        log,
		channels:   cfg.Channels,
		routes:     cfg.Routes,
		routeIndex: make(map[string][]int),
	}

	for i, r := range e.routes {
		for _, id := range r.Match.ServiceIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			e.routeIndex[id] = append(e.routeIndex[id], i)
		}
	}

	return e
}

func (e *Engine) RouteForService(serviceID string) (notify []string, policy *config.RoutePolicy, matched bool) {
	idxs := e.routeIndex[serviceID]
	if len(idxs) == 0 {
		return nil, nil, false
	}

	var out []string
	for j, idx := range idxs {
		r := e.routes[idx]
		if j == 0 {
			p := r.Policy
			policy = &p
		}
		for _, ch := range r.Notify {
			if ch == "" {
				continue
			}
			if !slices.Contains(out, ch) {
				out = append(out, ch)
			}
		}
	}
	return out, policy, true
}

func (e *Engine) HandleResult(svc config.Service, res checks.Result) {
	notify, policy, ok := e.RouteForService(svc.ID)
	if !ok || len(notify) == 0 {
		e.log.Debug("not alert route matched",
			"service_id", svc.ID,
			"service_name", svc.Name,
			"type", svc.Type,
		)
		return
	}

	var channelTypes []string
	for _, name := range notify {
		ch, exists := e.channels[name]
		if !exists {
			channelTypes = append(channelTypes, name+":(missing)")
			continue
		}
		channelTypes = append(channelTypes, name+":"+ch.Type)
	}

	args := []any{
		"service_id", svc.ID,
		"service_name", svc.Name,
		"type", svc.Type,
		"success", res.Success,
		"notify", notify,
		"channels", channelTypes,
	}

	if policy != nil {
		args = append(args,
			"policy_failure_threshold", policy.FailureThreshold,
			"policy_cooldown", policy.Cooldown,
			"policy_recovery_alert", policy.RecoveryAlert,
		)
	}

	e.log.Info("routing decision (stubbed)", args...)
}
