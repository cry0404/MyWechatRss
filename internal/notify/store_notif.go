package notify

import (
	"context"
	"log"

	"github.com/cry0404/MyWechatRss/internal/store"
)

type StoreNotifier struct {
	Store *store.Store
}

func NewStoreNotifier(st *store.Store) *StoreNotifier {
	return &StoreNotifier{Store: st}
}

func (n *StoreNotifier) sendViaSMTP(ctx context.Context) *SMTPNotifier {
	cfg, err := n.Store.GetSMTPConfig(ctx)
	if err != nil {
		log.Printf("[notify] read smtp config: %v", err)
		return nil
	}
	if cfg.Host == "" || cfg.Port == 0 {
		return nil
	}
	from := cfg.From
	if from == "" {
		from = cfg.Username
	}
	return NewSMTP(SMTPConfig{
		Host:     cfg.Host,
		Port:     cfg.Port,
		Username: cfg.Username,
		Password: cfg.Password,
		From:     from,
		UseTLS:   cfg.UseTLS,
	})
}

func (n *StoreNotifier) AccountsAllDead(ctx context.Context, ev AccountsDeadEvent) error {
	smtp := n.sendViaSMTP(ctx)
	if smtp == nil {
		return Noop{}.AccountsAllDead(ctx, ev)
	}
	return smtp.AccountsAllDead(ctx, ev)
}

func (n *StoreNotifier) FetchFailureAlert(ctx context.Context, ev FetchFailureAlertEvent) error {
	smtp := n.sendViaSMTP(ctx)
	if smtp == nil {
		return Noop{}.FetchFailureAlert(ctx, ev)
	}
	return smtp.FetchFailureAlert(ctx, ev)
}
