package notify

import (
	"context"
	"log"
	"time"
)

type AccountsDeadEvent struct {
	UserID    int64
	Username  string
	Email     string // 收件人。为空时 Notifier 应该静默丢弃。
	LastErr   string
	RescanURL string
}

type FetchFailureAlertEvent struct {
	Email      string
	FailRate   float64
	Threshold  float64
	WindowSec  int64
	CheckedAt  time.Time
}

type Notifier interface {
	AccountsAllDead(ctx context.Context, ev AccountsDeadEvent) error
	FetchFailureAlert(ctx context.Context, ev FetchFailureAlertEvent) error
}

type Noop struct{}

func (Noop) AccountsAllDead(_ context.Context, ev AccountsDeadEvent) error {
	log.Printf("[notify-noop] user %d (%s) all accounts dead; last_err=%q (SMTP not configured)",
		ev.UserID, ev.Username, ev.LastErr)
	return nil
}

func (Noop) FetchFailureAlert(_ context.Context, ev FetchFailureAlertEvent) error {
	log.Printf("[notify-noop] fetch failure alert: %.1f%% > %.1f%% (window=%ds, SMTP not configured)",
		ev.FailRate, ev.Threshold, ev.WindowSec)
	return nil
}
