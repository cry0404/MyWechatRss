package notify

import (
	"context"
	"log"
)

type AccountsDeadEvent struct {
	UserID   int64
	Username string
	Email    string // 收件人。为空时 Notifier 应该静默丢弃。
	LastErr string
	RescanURL string
}

type Notifier interface {
	AccountsAllDead(ctx context.Context, ev AccountsDeadEvent) error
}

type Noop struct{}

func (Noop) AccountsAllDead(_ context.Context, ev AccountsDeadEvent) error {
	log.Printf("[notify-noop] user %d (%s) all accounts dead; last_err=%q (SMTP not configured)",
		ev.UserID, ev.Username, ev.LastErr)
	return nil
}
