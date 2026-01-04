package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

var idPattern = "^[a-zA-Z0-9_-]+$"
var idRegex = regexp.MustCompile(idPattern)

func LoadAndValidateConfig(path string) (*SitelertConfig, error) {

	rawData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}

	expandedData := expandEnv(string(rawData))

	var cfg SitelertConfig
	if err := yaml.Unmarshal([]byte(expandedData), &cfg); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}

	applyDefaults(&cfg)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func applyDefaults(cfg *SitelertConfig) {
	if cfg.Global.ScrapeBind == "" {
		cfg.Global.ScrapeBind = "0.0.0.0:8080"
	}
	if cfg.Global.LogLevel == "" {
		cfg.Global.LogLevel = "info"
	}
	if cfg.Global.DefaultTimeout == "" {
		cfg.Global.DefaultTimeout = "5s"
	}
	if cfg.Global.DefaultInterval == "" {
		cfg.Global.DefaultInterval = "30s"
	}
	if cfg.Global.WorkerCount == 0 {
		cfg.Global.WorkerCount = 10
	}
	if cfg.Global.Jitter == "" {
		cfg.Global.Jitter = "0s"
	}

	for i := range cfg.Services {
		if cfg.Services[i].Timeout == "" {
			cfg.Services[i].Timeout = cfg.Global.DefaultTimeout
		}
		if cfg.Services[i].Interval == "" {
			cfg.Services[i].Interval = cfg.Global.DefaultInterval
		}
		if cfg.Services[i].Method == "" && strings.EqualFold(cfg.Services[i].Type, "http") {
			cfg.Services[i].Method = "GET"
		}
	}
}

func (cfg *SitelertConfig) Validate() error {
	var errs []string
	errs = append(errs, validateGlobalConfig(cfg.Global)...)

	seenIDs := map[string]struct{}{}
	for i, s := range cfg.Services {
		p := fmt.Sprintf("services[%d]", i)

		if s.ID == "" {
			errs = append(errs, p+".id is required")
		} else {
			if !isSafeID(s.ID) {
				errs = append(errs, fmt.Sprintf("%s.id %q contains invalid characters (use letters, numbers, _, -)", p, s.ID))
			}
			if _, ok := seenIDs[s.ID]; ok {
				errs = append(errs, fmt.Sprintf("%s.id %q is duplicated", p, s.ID))
			}
			seenIDs[s.ID] = struct{}{}
		}

		if s.Name == "" {
			errs = append(errs, p+".name is required")
		}

		switch strings.ToLower(s.Type) {
		case "http":
			if s.URL == "" {
				errs = append(errs, p+".url is required for type=http")
			}
		case "tcp":
			if s.Host == "" {
				errs = append(errs, p+".host is required for type=tcp")
			}
			if s.Port <= 0 || s.Port > 65535 {
				errs = append(errs, fmt.Sprintf("%s.port must be between 1 and 65535 for type=tcp (got %d)", p, s.Port))
			}
		default:
			errs = append(errs, fmt.Sprintf("%s.type must be either http or tcp (got %q)", p, s.Type))
		}

		if _, err := time.ParseDuration(s.Interval); err != nil {
			errs = append(errs, fmt.Sprintf("%s.interval must be a valid duration %q: %v", p, s.Interval, err))
		}

		if _, err := time.ParseDuration(s.Timeout); err != nil {
			errs = append(errs, fmt.Sprintf("%s.timeout must be a valid duration %q: %v", p, s.Timeout, err))
		}
	}

	for name, ch := range cfg.Alerting.Channels {
		if name == "" {
			errs = append(errs, "alerting.channels contains an empty name")
			continue
		}
		switch strings.ToLower(ch.Type) {
		case "":
			// don't force alerting
		case "discord", "slack":
			if strings.TrimSpace(ch.WebhookURL) == "" {
				errs = append(errs, fmt.Sprintf("alerting.channels[%q].webhook_url is required for type=%q", name, ch.Type))
			}
		case "email":
			if strings.TrimSpace(ch.SMTPHost) == "" {
				errs = append(errs, fmt.Sprintf("alerting.channels[%q].smtp_host is required for type=email", name))
			}
			if ch.SMTPPort == 0 {
				errs = append(errs, fmt.Sprintf("alerting.channels[%q].smtp_port is required for type=email", name))
			}
			if strings.TrimSpace(ch.From) == "" {
				errs = append(errs, fmt.Sprintf("alerting.channels[%q].from is required for type=email", name))
			}
			if len(ch.To) == 0 {
				errs = append(errs, fmt.Sprintf("alerting.channels[%q].to is required for type=email", name))
			}
		default:
			errs = append(errs, fmt.Sprintf("alerting.channels[%q].type must be one of: discord, slack, email (got %q)", name, ch.Type))
		}
	}

	if len(cfg.Alerting.Routes) > 0 && len(cfg.Alerting.Channels) == 0 {
		errs = append(errs, "alerting.routes defined but no alerting.channels present")
	}
	for i, r := range cfg.Alerting.Routes {
		p := fmt.Sprintf("alerting.routes[%d]", i)
		for _, chName := range r.Notify {
			if _, ok := cfg.Alerting.Channels[chName]; !ok {
				errs = append(errs, fmt.Sprintf("%s.notify references undefined channel %q", p, chName))
			}
		}
		if r.Policy.Cooldown != "" {
			if _, err := time.ParseDuration(r.Policy.Cooldown); err != nil {
				errs = append(errs, fmt.Sprintf("%s.policy.cooldown must be a valid duration %q: %v", p, r.Policy.Cooldown, err))
			}
		}
	}

	if len(errs) > 0 {
		sort.Strings(errs)
		return errors.New("config validation failed:\n- " + strings.Join(errs, "\n- "))
	}
	return nil
}

func validateGlobalConfig(global GlobalConfig) []string {
	var errs []string
	if _, _, err := net.SplitHostPort(global.ScrapeBind); err != nil {
		errs = append(errs, fmt.Sprintf("global.scrape_bind must be a valid host:port (got %q): %v", global.ScrapeBind, err))
	}
	if _, err := time.ParseDuration(global.DefaultTimeout); err != nil {
		errs = append(errs, fmt.Sprintf("global.default_timeout must be a valid duration %q: %v", global.DefaultTimeout, err))
	}
	if _, err := time.ParseDuration(global.DefaultInterval); err != nil {
		errs = append(errs, fmt.Sprintf("global.default_interval must be a valid duration %q: %v", global.DefaultInterval, err))
	}
	if _, err := time.ParseDuration(global.Jitter); err != nil {
		errs = append(errs, fmt.Sprintf("global.jitter must be a valid duration %q: %v", global.Jitter, err))
	}
	if global.WorkerCount < 1 || global.WorkerCount > 1000 {
		errs = append(errs, fmt.Sprintf("global.worker_count must be between 1 and 1000 (got %d)", global.WorkerCount))
	}
	return errs
}

func isSafeID(id string) bool {
	return idRegex.MatchString(id)
}

func expandEnv(s string) string {
	return os.Expand(s, func(key string) string {
		return os.Getenv(key)
	})
}
