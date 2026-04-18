package notify

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string // 发件人。为空时会用 Username。
	UseTLS bool
}

func NewSMTP(cfg SMTPConfig) *SMTPNotifier {
	from := strings.TrimSpace(cfg.From)
	if from == "" {
		from = cfg.Username
	}
	return &SMTPNotifier{cfg: cfg, from: from}
}

type SMTPNotifier struct {
	cfg  SMTPConfig
	from string
}

func (s *SMTPNotifier) AccountsAllDead(ctx context.Context, ev AccountsDeadEvent) error {
	if ev.Email == "" {
		log.Printf("[notify-smtp] user %d has no email, skip", ev.UserID)
		return nil
	}
	subject := "[WeChatRead RSS] 所有微信读书账号已失效"
	body := buildAllDeadBody(ev)
	return s.send(ctx, ev.Email, subject, body)
}

func (s *SMTPNotifier) FetchFailureAlert(ctx context.Context, ev FetchFailureAlertEvent) error {
	if ev.Email == "" {
		log.Printf("[notify-smtp] fetch failure alert: no email, skip")
		return nil
	}
	subject := "[WeChatRead RSS] 正文抓取失败率告警"
	body := buildFetchFailureBody(ev)
	return s.send(ctx, ev.Email, subject, body)
}

func buildFetchFailureBody(ev FetchFailureAlertEvent) string {
	var b strings.Builder
	b.WriteString("你好，\n\n")
	fmt.Fprintf(&b, "最近 %d 分钟内正文抓取失败率为 %.1f%%，已超过告警阈值 %.1f%%。\n\n", ev.WindowSec/60, ev.FailRate, ev.Threshold)
	b.WriteString("可能原因：\n")
	b.WriteString("  - 微信读书网页端接口风控\n")
	b.WriteString("  - 微信公众号公开页面反爬\n")
	b.WriteString("  - 所有微信读书账号凭证失效\n\n")
	b.WriteString("建议检查：\n")
	b.WriteString("  1. 微信读书账号状态\n")
	b.WriteString("  2. 服务器网络环境\n")
	b.WriteString("  3. CONTENT_FETCH_MODE 是否设为 summary 以暂停正文抓取\n\n")
	b.WriteString("（本邮件由自部署的 WeChatRead-RSS 自动发送，无需回复。）\n")
	return b.String()
}

func buildAllDeadBody(ev AccountsDeadEvent) string {
	var b strings.Builder
	b.WriteString("你好 ")
	b.WriteString(ev.Username)
	b.WriteString("，\n\n")
	b.WriteString("你绑定的所有微信读书账号现在都已失效，RSS 抓取将暂停直到你重新扫码。\n\n")
	if ev.LastErr != "" {
		b.WriteString("最近一次失败原因：")
		b.WriteString(ev.LastErr)
		b.WriteString("\n\n")
	}
	if ev.RescanURL != "" {
		b.WriteString("重新扫码入口：")
		b.WriteString(ev.RescanURL)
		b.WriteString("\n\n")
	}
	b.WriteString("（本邮件由自部署的 WeChatRead-RSS 自动发送，无需回复。）\n")
	return b.String()
}

func (s *SMTPNotifier) send(ctx context.Context, to, subject, body string) error {
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	msg := buildRFC822(s.from, to, subject, body)

	var auth smtp.Auth
	if s.cfg.Username != "" || s.cfg.Password != "" {
		auth = smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	}

	if !s.cfg.UseTLS {
		errc := make(chan error, 1)
		go func() {
			errc <- smtp.SendMail(addr, auth, s.from, []string{to}, msg)
		}()
		select {
		case err := <-errc:
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Second):
			return fmt.Errorf("smtp send timeout to %s", addr)
		}
	}

	d := &net.Dialer{Timeout: 15 * time.Second}
	conn, err := tls.DialWithDialer(d, "tcp", addr, &tls.Config{ServerName: s.cfg.Host})
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("smtp new client: %w", err)
	}
	defer c.Quit()
	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := c.Mail(s.from); err != nil {
		return fmt.Errorf("smtp MAIL: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("smtp RCPT: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close data: %w", err)
	}
	return nil
}

func buildRFC822(from, to, subject, body string) []byte {
	var b strings.Builder
	b.WriteString("From: ")
	b.WriteString(from)
	b.WriteString("\r\n")
	b.WriteString("To: ")
	b.WriteString(to)
	b.WriteString("\r\n")
	b.WriteString("Subject: ")
	b.WriteString(encodeRFC2047(subject))
	b.WriteString("\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return []byte(b.String())
}

func encodeRFC2047(s string) string {
	if isASCII(s) {
		return s
	}
	return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(s)) + "?="
}

func isASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}
