package alerting

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"sitelert/internal/config"
	"strings"
	"time"
)

func (e *Engine) sendEmail(ctx context.Context, ch config.Channel, subject, body string) error {
	if strings.TrimSpace(ch.SMTPHost) == "" {
		return fmt.Errorf("smtp_host is empty")
	}
	if ch.SMTPPort == 0 {
		return fmt.Errorf("smtp_port is empty/0")
	}
	if strings.TrimSpace(ch.From) == "" {
		return fmt.Errorf("from is empty")
	}
	if len(ch.To) == 0 {
		return fmt.Errorf("to list is empty")
	}

	fromHdr := ch.From
	fromAddr, err := parseAddress(fromHdr)
	if err != nil {
		return fmt.Errorf("parse from: %w", err)
	}

	var toHdrs []string
	var toAddrs []string
	for _, t := range ch.To {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		addr, err := parseAddress(t)
		if err != nil {
			return fmt.Errorf("parse to %q: %w", t, err)
		}
		toHdrs = append(toHdrs, t)
		toAddrs = append(toAddrs, addr)
	}
	if len(toAddrs) == 0 {
		return fmt.Errorf("no valid recipients in to list")
	}

	// Build message (RFC 5322-ish)
	msg := buildEmail(fromHdr, toHdrs, subject, body)

	// Dial with context
	addr := net.JoinHostPort(ch.SMTPHost, fmt.Sprintf("%d", ch.SMTPPort))
	dialer := &net.Dialer{Timeout: 7 * time.Second}

	var conn net.Conn
	done := make(chan error, 1)
	go func() {
		c, err := dialer.Dial("tcp", addr)
		if err != nil {
			done <- err
			return
		}
		conn = c
		done <- nil
	}()

	select {
	case <-ctx.Done():
		if conn != nil {
			_ = conn.Close()
		}
		return ctx.Err()
	case err := <-done:
		if err != nil {
			return fmt.Errorf("dial smtp: %w", err)
		}
	}

	// Ensure conn closed on failure
	defer func() {
		if conn != nil {
			_ = conn.Close()
		}
	}()

	host := ch.SMTPHost

	// Port 465: implicit TLS
	implicitTLS := ch.SMTPPort == 465
	if implicitTLS {
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName: host,
			MinVersion: tls.VersionTLS12,
		})
		if err := tlsConn.Handshake(); err != nil {
			return fmt.Errorf("tls handshake: %w", err)
		}
		conn = tlsConn
	}

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	// Once we have smtp client, it owns the conn; avoid double close confusion
	conn = nil
	defer func() { _ = c.Close() }()

	// STARTTLS if available (and not already implicit TLS)
	isTLS := implicitTLS
	if !implicitTLS {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{
				ServerName: host,
				MinVersion: tls.VersionTLS12,
			}); err != nil {
				return fmt.Errorf("starttls: %w", err)
			}
			isTLS = true
		}
	}

	// If auth is configured, refuse to send creds without TLS
	authConfigured := strings.TrimSpace(ch.Username) != "" || strings.TrimSpace(ch.Password) != ""
	if authConfigured && !isTLS {
		return fmt.Errorf("refusing to authenticate without TLS (enable STARTTLS or use port 465)")
	}

	// Authenticate if username provided
	if strings.TrimSpace(ch.Username) != "" {
		auth := smtp.PlainAuth("", ch.Username, ch.Password, host)
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	// Envelope
	if err := c.Mail(fromAddr); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	for _, rcpt := range toAddrs {
		if err := c.Rcpt(rcpt); err != nil {
			return fmt.Errorf("rcpt to %s: %w", rcpt, err)
		}
	}

	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return fmt.Errorf("write data: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close data: %w", err)
	}

	// Quit politely
	_ = c.Quit()
	return nil
}

func parseAddress(s string) (string, error) {
	a, err := mail.ParseAddress(strings.TrimSpace(s))
	if err != nil {
		return "", err
	}
	return a.Address, nil
}

func buildEmail(from string, to []string, subject, body string) []byte {
	// Keep it simple: text/plain UTF-8.
	// Use CRLF line endings for SMTP.
	var b bytes.Buffer
	writeHeader(&b, "From", from)
	writeHeader(&b, "To", strings.Join(to, ", "))
	writeHeader(&b, "Subject", sanitizeHeader(subject))
	writeHeader(&b, "Date", time.Now().Format(time.RFC1123Z))
	writeHeader(&b, "MIME-Version", "1.0")
	writeHeader(&b, "Content-Type", `text/plain; charset="utf-8"`)
	writeHeader(&b, "Content-Transfer-Encoding", "8bit")
	b.WriteString("\r\n")

	// Body (normalize line endings)
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	body = strings.ReplaceAll(body, "\n", "\r\n")
	b.WriteString(body)
	if !strings.HasSuffix(body, "\r\n") {
		b.WriteString("\r\n")
	}
	return b.Bytes()
}

func writeHeader(b *bytes.Buffer, k, v string) {
	b.WriteString(k)
	b.WriteString(": ")
	b.WriteString(v)
	b.WriteString("\r\n")
}

func sanitizeHeader(s string) string {
	// Prevent header injection
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}
