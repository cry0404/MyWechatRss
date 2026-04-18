package store

import (
	"context"
	"database/sql"
	"strconv"
)

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	UseTLS   bool
}

const (
	cfgSMTPHost     = "smtp_host"
	cfgSMTPPort     = "smtp_port"
	cfgSMTPUsername = "smtp_username"
	cfgSMTPPassword = "smtp_password"
	cfgSMTPFrom     = "smtp_from"
	cfgSMTPUseTLS   = "smtp_use_tls"
)

func (s *Store) setConfigString(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO site_config (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}

func (s *Store) getConfigString(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM site_config WHERE key = ?`, key).Scan(&v)
	if err != nil {
		return "", err
	}
	return v, nil
}

func (s *Store) PutSMTPConfig(ctx context.Context, cfg SMTPConfig) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_ = setTx(tx, cfgSMTPHost, cfg.Host)
		_ = setTx(tx, cfgSMTPPort, strconv.Itoa(cfg.Port))
		_ = setTx(tx, cfgSMTPUsername, cfg.Username)
		_ = setTx(tx, cfgSMTPPassword, cfg.Password)
		_ = setTx(tx, cfgSMTPFrom, cfg.From)
		tlsStr := "0"
		if cfg.UseTLS {
			tlsStr = "1"
		}
		_ = setTx(tx, cfgSMTPUseTLS, tlsStr)
		return nil
	})
}

func (s *Store) GetSMTPConfig(ctx context.Context) (*SMTPConfig, error) {
	get := func(key string) string {
		v, _ := s.getConfigString(ctx, key)
		return v
	}
	port, _ := strconv.Atoi(get(cfgSMTPPort))
	tls := get(cfgSMTPUseTLS) == "1"
	return &SMTPConfig{
		Host:     get(cfgSMTPHost),
		Port:     port,
		Username: get(cfgSMTPUsername),
		Password: get(cfgSMTPPassword),
		From:     get(cfgSMTPFrom),
		UseTLS:   tls,
	}, nil
}

func setTx(tx *sql.Tx, key, value string) error {
	_, err := tx.Exec(`
		INSERT INTO site_config (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}
