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

func (n *StoreNotifier) AccountsAllDead(ctx context.Context, ev AccountsDeadEvent) error {
	cfg, err := n.Store.GetSMTPConfig(ctx)
	if err != nil {
		log.Printf("[notify] read smtp config: %v", err)
		return Noop{}.AccountsAllDead(ctx, ev)
	}
	if cfg.Host == "" || cfg.Port == 0 {
		return Noop{}.AccountsAllDead(ctx, ev)
	}

	from := cfg.From
	if from == "" {
		from = cfg.Username
	}

	smtp := NewSMTP(SMTPConfig{
		Host:     cfg.Host,
		Port:     cfg.Port,
		Username: cfg.Username,
		Password: cfg.Password,
		From:     from,
		UseTLS:   cfg.UseTLS,
	})
	return smtp.AccountsAllDead(ctx, ev)
}
