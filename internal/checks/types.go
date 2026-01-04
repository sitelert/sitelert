package checks

import "time"

type Result struct {
	Success    bool
	StatusCode int
	Latency    time.Duration
	Error      string
}
