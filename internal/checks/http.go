package checks

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"sitelert/internal/config"
	"strings"
	"time"
)

type HTTPChecker struct {
	client *http.Client
}

func NewHTTPChecker() *HTTPChecker {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}

	return &HTTPChecker{
		client: &http.Client{
			Transport: transport,
		},
	}
}

func (h *HTTPChecker) Check(ctx context.Context, svc config.Service) Result {
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(svc.Method), svc.URL, nil)
	if err != nil {
		return Result{Success: false, Latency: time.Since(start), Error: fmt.Sprintf("build request: %v", err)}
	}

	for k, v := range svc.Headers {
		req.Header.Set(k, v)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return Result{Success: false, Latency: time.Since(start), Error: err.Error()}
	}
	defer resp.Body.Close()

	res := Result{
		StatusCode: resp.StatusCode,
		Latency:    time.Since(start),
		Success:    true,
	}

	if len(svc.ExpectedStatus) > 0 {
		allowed := false
		for _, code := range svc.ExpectedStatus {
			if resp.StatusCode == code {
				allowed = true
				break
			}
		}
		if !allowed {
			res.Success = false
			res.Error = fmt.Sprintf("unexpected status %d", resp.StatusCode)
			return res
		}
	} else {
		if resp.StatusCode < 200 || resp.StatusCode >= 400 {
			res.Success = false
			res.Error = fmt.Sprintf("unexpected status %d", resp.StatusCode)
			return res
		}
	}

	if strings.TrimSpace(svc.Contains) != "" {
		const maxBody = 1024 * 1024 // 1 MiB
		b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
		if err != nil {
			res.Success = false
			res.Error = fmt.Sprintf("read body: %v", err)
			return res
		}
		if !strings.Contains(string(b), svc.Contains) {
			res.Success = false
			res.Error = "response does not contain expected content"
			return res
		}
	}

	return res
}
