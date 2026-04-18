package articles

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"github.com/cry0404/MyWechatRss/internal/notify"
	"github.com/cry0404/MyWechatRss/internal/store"
)

type Alerter struct {
	Store     *store.Store
	Notifier  notify.Notifier

	CheckInterval   time.Duration
	WindowSec       int64
	ThresholdPct    float64
	CooldownDuration time.Duration

	lastAlertAt atomic.Int64 // unix timestamp
}

func NewAlerter(st *store.Store, notifier notify.Notifier) *Alerter {
	return &Alerter{
		Store:            st,
		Notifier:         notifier,
		CheckInterval:    15 * time.Minute,
		WindowSec:        1800, // 30 min
		ThresholdPct:     80.0,
		CooldownDuration: 60 * time.Minute,
	}
}

func (a *Alerter) Run(ctx context.Context) {
	if a.CheckInterval <= 0 {
		a.CheckInterval = 15 * time.Minute
	}
	t := time.NewTicker(a.CheckInterval)
	defer t.Stop()
	log.Printf("fetch alerter started, check_interval=%s, window=%ds, threshold=%.1f%%",
		a.CheckInterval, a.WindowSec, a.ThresholdPct)

	a.checkOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.checkOnce(ctx)
		}
	}
}

func (a *Alerter) checkOnce(ctx context.Context) {
	failRate, err := a.Store.GetRecentFailureRate(ctx, a.WindowSec)
	if err != nil {
		log.Printf("alerter: get failure rate: %v", err)
		return
	}
	if failRate < a.ThresholdPct {
		if a.lastAlertAt.Load() > 0 {
			log.Printf("alerter: failure rate %.1f%% back to normal (threshold %.1f%%)", failRate, a.ThresholdPct)
			a.lastAlertAt.Store(0)
		}
		return
	}

	now := time.Now()
	lastAlert := time.Unix(a.lastAlertAt.Load(), 0)
	if now.Sub(lastAlert) < a.CooldownDuration {
		log.Printf("alerter: failure rate %.1f%% > threshold %.1f%%, but in cooldown", failRate, a.ThresholdPct)
		return
	}

	log.Printf("alerter: failure rate %.1f%% > threshold %.1f%%, sending alert", failRate, a.ThresholdPct)
	a.lastAlertAt.Store(now.Unix())
	a.sendAlert(ctx, failRate)
}

func (a *Alerter) sendAlert(ctx context.Context, failRate float64) {
	// 获取所有管理员用户发送告警
	users, err := a.Store.ListAdminUsers(ctx)
	if err != nil {
		log.Printf("alerter: list admin users: %v", err)
		return
	}
	if len(users) == 0 {
		log.Printf("alerter: no admin users found, skip alert")
		return
	}

	for _, u := range users {
		if u.Email == "" {
			continue
		}
		ev := notify.FetchFailureAlertEvent{
			Email:     u.Email,
			FailRate:  failRate,
			Threshold: a.ThresholdPct,
			WindowSec: a.WindowSec,
			CheckedAt: time.Now(),
		}
		if err := a.Notifier.FetchFailureAlert(ctx, ev); err != nil {
			log.Printf("alerter: notify admin %d (%s): %v", u.ID, u.Username, err)
		}
	}
}
