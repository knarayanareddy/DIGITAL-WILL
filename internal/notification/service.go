package notification

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"github.com/knarayanareddy/DIGITAL-WILL/internal/action"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/will"
)

type Service struct {
	userName   string
	requireTLS bool
}

func New(userName string, requireTLS bool) *Service {
	return &Service{userName: userName, requireTLS: requireTLS}
}

func (s *Service) Execute(typ action.Type, cfg *action.Config, payload *will.Payload, w *will.Will) error {
	switch typ {
	case action.TypeSMTP:
		return s.sendSMTP(cfg, payload, w)
	case action.TypeWebhook:
		return s.sendWebhook(cfg, payload, w)
	case action.TypeScript:
		return s.runScript(cfg, payload, w)
	case action.TypeSignal:
		return fmt.Errorf("signal action type not implemented — see KNOWN_GAPS.md")
	default:
		return fmt.Errorf("unknown action type: %s", typ)
	}
}

func (s *Service) renderTemplate(tpl string, payload *will.Payload, w *will.Will) (string, error) {
	lastCheck := "never"
	if w.LastCheckIn != nil {
		lastCheck = time.Unix(*w.LastCheckIn, 0).Format(time.RFC3339)
	}

	data := map[string]string{
		"user_name":    s.userName,
		"will_name":    w.Name,
		"will_id":      w.ID,
		"triggered_at": time.Now().Format(time.RFC3339),
		"last_check_in": lastCheck,
		"content":      payload.Content,
	}

	t, err := template.New("msg").Parse(tpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (s *Service) sendSMTP(cfg *action.Config, payload *will.Payload, w *will.Will) error {
	if cfg.TLS == "none" && s.requireTLS {
		return errors.New("TLS required but action configured as none")
	}

	subject, _ := s.renderTemplate(cfg.Subject, payload, w)
	body, _ := s.renderTemplate(cfg.Body, payload, w)

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s", cfg.From, cfg.To, subject, body)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	if cfg.TLS == "tls" {
		tlsConfig := &tls.Config{
			ServerName: cfg.Host,
			MinVersion: tls.VersionTLS12,
		}
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return err
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, cfg.Host)
		if err != nil {
			return err
		}
		defer client.Close()

		if err := client.Auth(smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)); err != nil {
			return err
		}
		if err := client.Mail(cfg.From); err != nil {
			return err
		}
		if err := client.Rcpt(cfg.To); err != nil {
			return err
		}
		wc, err := client.Data()
		if err != nil {
			return err
		}
		_, err = wc.Write([]byte(msg))
		wc.Close()
		return err
	}

	if cfg.TLS == "starttls" {
		client, err := smtp.Dial(addr)
		if err != nil {
			return err
		}
		defer client.Close()

		if ok, _ := client.Extension("STARTTLS"); !ok && s.requireTLS {
			return errors.New("server does not support STARTTLS")
		}
		tlsConfig := &tls.Config{ServerName: cfg.Host}
		if err := client.StartTLS(tlsConfig); err != nil {
			return err
		}
		if err := client.Auth(smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)); err != nil {
			return err
		}
		if err := client.Mail(cfg.From); err != nil {
			return err
		}
		if err := client.Rcpt(cfg.To); err != nil {
			return err
		}
		wc, err := client.Data()
		if err != nil {
			return err
		}
		_, err = wc.Write([]byte(msg))
		wc.Close()
		return err
	}

	// plain (only if requireTLS == false)
	auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	return smtp.SendMail(addr, auth, cfg.From, []string{cfg.To}, []byte(msg))
}

func (s *Service) sendWebhook(cfg *action.Config, payload *will.Payload, w *will.Will) error {
	body, _ := s.renderTemplate(cfg.Body, payload, w)

	client := &http.Client{Timeout: 30 * time.Second}
	if !cfg.TLSVerify {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client.Transport = tr
	}

	req, err := http.NewRequest(cfg.Method, cfg.URL, strings.NewReader(body))
	if err != nil {
		return err
	}
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook failed with status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func (s *Service) runScript(cfg *action.Config, payload *will.Payload, w *will.Will) error {
	timeout := cfg.TimeoutSec
	if timeout == 0 {
		timeout = 30
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	args := make([]string, len(cfg.Args))
	for i, a := range cfg.Args {
		rendered, _ := s.renderTemplate(a, payload, w)
		args[i] = rendered
	}

	cmd := exec.CommandContext(ctx, cfg.Command, args...)

	env := make([]string, 0, len(cfg.Env))
	for k, v := range cfg.Env {
		rendered, _ := s.renderTemplate(v, payload, w)
		env = append(env, fmt.Sprintf("%s=%s", k, rendered))
	}
	cmd.Env = env

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("script failed: %v (output: %s)", err, string(output[:min(500, len(output))]))
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}