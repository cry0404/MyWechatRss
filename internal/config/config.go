package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddr string

	DBPath string

	AppSecret string

	JWTSecret string

	UpstreamBaseURL  string
	UpstreamAPIKeyID string
	UpstreamAPISecret string

	PublicBaseURL string

	FeedIDSalt string

	AllowRegister bool

	BootstrapUsername string
	BootstrapPassword string
	BootstrapEmail    string

	DefaultDeviceName string

	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
	SMTPUseTLS   bool
}

func (c *Config) SMTPEnabled() bool {
	return c.SMTPHost != "" && c.SMTPPort > 0
}

func Load() (*Config, error) {
	cfg := &Config{
		ListenAddr:        getEnv("LISTEN_ADDR", ":8081"),
		DBPath:            getEnv("DB_PATH", "./data/app.db"),
		AppSecret:         os.Getenv("APP_SECRET"),
		JWTSecret:         os.Getenv("JWT_SECRET"),
		UpstreamBaseURL:   strings.TrimRight(os.Getenv("UPSTREAM_BASE_URL"), "/"),
		UpstreamAPIKeyID:  os.Getenv("UPSTREAM_API_KEY_ID"),
		UpstreamAPISecret: os.Getenv("UPSTREAM_API_SECRET"),
		PublicBaseURL:     strings.TrimRight(getEnv("PUBLIC_BASE_URL", "http://localhost:8081"), "/"),
		FeedIDSalt:        getEnv("FEED_ID_SALT", "wechatread-rss"),
		AllowRegister:     getEnvBool("ALLOW_REGISTER", false),
		DefaultDeviceName: getEnv("DEFAULT_DEVICE_NAME", "wechatread-rss"),
		BootstrapUsername: getEnv("BOOTSTRAP_USERNAME", "admin"),
		BootstrapPassword: getEnv("BOOTSTRAP_PASSWORD", "changeme"),
		BootstrapEmail:    getEnv("BOOTSTRAP_EMAIL", "admin@local.invalid"),

		SMTPHost:     os.Getenv("SMTP_HOST"),
		SMTPPort:     getEnvInt("SMTP_PORT", 0),
		SMTPUsername: os.Getenv("SMTP_USERNAME"),
		SMTPPassword: os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:     os.Getenv("SMTP_FROM"),
		SMTPUseTLS:   getEnvBool("SMTP_USE_TLS", false),
	}

	var missing []string
	if cfg.AppSecret == "" {
		missing = append(missing, "APP_SECRET")
	}
	if cfg.JWTSecret == "" {
		missing = append(missing, "JWT_SECRET")
	}
	if cfg.UpstreamBaseURL == "" {
		missing = append(missing, "UPSTREAM_BASE_URL")
	}
	if cfg.UpstreamAPIKeyID == "" {
		missing = append(missing, "UPSTREAM_API_KEY_ID")
	}
	if cfg.UpstreamAPISecret == "" {
		missing = append(missing, "UPSTREAM_API_SECRET")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required env vars: %s", strings.Join(missing, ", "))
	}
	if len(cfg.AppSecret) < 16 {
		return nil, errors.New("APP_SECRET too short (>= 16 chars required)")
	}
	return cfg, nil
}

func getEnv(k, def string) string {
	if v, ok := os.LookupEnv(k); ok && v != "" {
		return v
	}
	return def
}

func getEnvInt(k string, def int) int {
	v, ok := os.LookupEnv(k)
	if !ok || v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func getEnvBool(k string, def bool) bool {
	v, ok := os.LookupEnv(k)
	if !ok || v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
