package config

type SitelertConfig struct {
	Global   GlobalConfig   `yaml:"global"`
	Services []Service      `yaml:"services"`
	Alerting AlertingConfig `yaml:"alerting"`
}

type GlobalConfig struct {
	ScrapeBind      string `yaml:"scrape_bind"`
	LogLevel        string `yaml:"log_level"`
	DefaultTimeout  string `yaml:"default_timeout"`
	DefaultInterval string `yaml:"default_interval"`
	WorkerCount     int    `yaml:"worker_count"`
	Jitter          string `yaml:"jitter"`
}

type Service struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
	Type string `yaml:"type"` // "http" or "tcp" (validate)
	// HTTP
	URL            string            `yaml:"url"`
	Method         string            `yaml:"method"`
	ExpectedStatus []int             `yaml:"expected_status"`
	Contains       string            `yaml:"contains"`
	Headers        map[string]string `yaml:"headers"`
	// TCP
	Host string `yaml:"host"`
	Port int    `yaml:"port"`

	Interval string `yaml:"interval"`
	Timeout  string `yaml:"timeout"`
}

type AlertingConfig struct {
	Channels map[string]Channel `yaml:"channels"`
	Routes   []Route            `yaml:"routes"`
}

// Channel supports multiple types; keep a superset of fields.
type Channel struct {
	Type       string   `yaml:"type"` // "discord" | "slack" | "email"
	WebhookURL string   `yaml:"webhook_url"`
	SMTPHost   string   `yaml:"smtp_host"`
	SMTPPort   int      `yaml:"smtp_port"`
	Username   string   `yaml:"username"`
	Password   string   `yaml:"password"`
	From       string   `yaml:"from"`
	To         []string `yaml:"to"`
}

type Route struct {
	Match  RouteMatch  `yaml:"match"`
	Policy RoutePolicy `yaml:"policy"`
	Notify []string    `yaml:"notify"`
}

type RouteMatch struct {
	ServiceIDs []string `yaml:"service_ids"`
}

type RoutePolicy struct {
	FailureThreshold int    `yaml:"failure_threshold"`
	Cooldown         string `yaml:"cooldown"`
	RecoveryAlert    bool   `yaml:"recovery_alert"`
}
