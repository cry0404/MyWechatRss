package articles

import (
	"context"
	"errors"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/cry0404/MyWechatRss/internal/accounts"
	"github.com/cry0404/MyWechatRss/internal/model"
	"github.com/cry0404/MyWechatRss/internal/store"
)

type Scheduler struct {
	Store   *store.Store
	Service *Service

	Tick time.Duration

	InterSubSleepMin time.Duration
	InterSubSleepMax time.Duration

	MaxSubsPerBatch     int
	BatchCooldownMin    time.Duration
	BatchCooldownMax    time.Duration
	DeferredSubSpacing  time.Duration
	RateLimitBackoffMin time.Duration
	RateLimitBackoffMax time.Duration

	warnedUsers sync.Map // map[int64]struct{}
}

func NewScheduler(st *store.Store, svc *Service) *Scheduler {
	return &Scheduler{
		Store:               st,
		Service:             svc,
		Tick:                time.Minute,
		InterSubSleepMin:    30 * time.Second,
		InterSubSleepMax:    120 * time.Second,
		MaxSubsPerBatch:     2,
		BatchCooldownMin:    30 * time.Minute,
		BatchCooldownMax:    60 * time.Minute,
		DeferredSubSpacing:  10 * time.Minute,
		RateLimitBackoffMin: 4 * time.Hour,
		RateLimitBackoffMax: 6 * time.Hour,
	}
}

func (s *Scheduler) Run(ctx context.Context) {
	if s.Tick <= 0 {
		s.Tick = time.Minute
	}
	t := time.NewTicker(s.Tick)
	defer t.Stop()
	log.Printf("fetch scheduler started, tick=%s", s.Tick)

	s.runOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.runOnce(ctx)
		}
	}
}

func (s *Scheduler) runOnce(ctx context.Context) {
	subs, err := s.Store.ListSubscriptionsDueForFetch(ctx, time.Now().Unix())
	if err != nil {
		log.Printf("fetch: list due subs: %v", err)
		return
	}

	runnable := subs[:0]
	seenUsers := map[int64]bool{}
	for _, sub := range subs {
		uid := sub.UserID
		if muted, ok := seenUsers[uid]; ok {
			if !muted {
				runnable = append(runnable, sub)
			}
			continue
		}
		n, err := s.Store.CountActiveAccounts(ctx, uid)
		if err != nil {
			log.Printf("fetch: count active accounts for user %d: %v", uid, err)
			seenUsers[uid] = true // 查错按"静默"处理，下轮再试
			continue
		}
		if n == 0 {
			seenUsers[uid] = true
			if _, alreadyWarned := s.warnedUsers.LoadOrStore(uid, struct{}{}); !alreadyWarned {
				log.Printf("fetch: user %d has no active weread account, pausing scheduler for this user until cooldown expires or rescan", uid)
			}
			continue
		}
		s.warnedUsers.Delete(uid)
		seenUsers[uid] = false
		runnable = append(runnable, sub)
	}

	now := time.Now()
	mod := minuteOfDayLocal(now)
	runnable = filterByFetchWindow(runnable, mod)
	runnable = s.limitRecoveryProbe(ctx, runnable)
	runnable = s.limitByBatch(ctx, runnable, now)

	blockedUsers := map[int64]bool{}
	for i, sub := range runnable {
		if blockedUsers[sub.UserID] {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		if i > 0 {
			s.interSubSleep(ctx)
			if ctx.Err() != nil {
				return
			}
		}
		n, err := s.Service.FetchLatest(ctx, sub.UserID, sub.ID)
		if err != nil {
			if errors.Is(err, accounts.ErrHighRiskDeferred) {
				s.deferAllForUser(ctx, sub.UserID, accounts.AuthRefreshRetryDelay, "credential refresh", 0)
				blockedUsers[sub.UserID] = true
				continue
			}
			if errors.Is(err, accounts.ErrSearchRateLimited) {
				s.deferAllForUser(ctx, sub.UserID, s.rateLimitBackoff(), "search rate limit", sub.ID)
				blockedUsers[sub.UserID] = true
				continue
			}
			if errors.Is(err, accounts.ErrNoAccount) {
				continue
			}
			log.Printf("fetch sub %d (%s): %v", sub.ID, sub.BookID, err)
			continue
		}
		if n > 0 {
			log.Printf("fetch sub %d (%s): %d new", sub.ID, sub.BookID, n)
		}
	}
}

func (s *Scheduler) limitRecoveryProbe(ctx context.Context, subs []*model.Subscription) []*model.Subscription {
	if len(subs) == 0 {
		return subs
	}
	now := time.Now()
	out := subs[:0]
	selected := map[int64]bool{}
	skipped := map[int64][]int64{}
	for _, sub := range subs {
		recovering, err := s.Store.HasRateLimitRecoveryAccount(ctx, sub.UserID)
		if err != nil {
			log.Printf("fetch: check rate-limit recovery user=%d: %v", sub.UserID, err)
			out = append(out, sub)
			continue
		}
		if !recovering {
			out = append(out, sub)
			continue
		}
		if selected[sub.UserID] {
			skipped[sub.UserID] = append(skipped[sub.UserID], sub.ID)
			continue
		}
		selected[sub.UserID] = true
		out = append(out, sub)
	}
	for uid, ids := range skipped {
		next := now.Add(s.batchCooldown())
		if err := s.Store.DeferSubscriptionsFetch(ctx, ids, next.Unix(), s.deferredSubSpacing()); err != nil {
			log.Printf("fetch: user %d defer recovery overflow subscriptions: %v", uid, err)
			continue
		}
		log.Printf("fetch: user %d rate-limit recovery probe selected 1 subscription, deferred %d until %s",
			uid, len(ids), next.Format(time.RFC3339))
	}
	return out
}

func (s *Scheduler) limitByBatch(ctx context.Context, subs []*model.Subscription, now time.Time) []*model.Subscription {
	if s.MaxSubsPerBatch <= 0 || len(subs) == 0 {
		return subs
	}

	out := subs[:0]
	counts := map[int64]int{}
	skipped := map[int64][]int64{}
	for _, sub := range subs {
		if counts[sub.UserID] >= s.MaxSubsPerBatch {
			skipped[sub.UserID] = append(skipped[sub.UserID], sub.ID)
			continue
		}
		counts[sub.UserID]++
		out = append(out, sub)
	}

	for uid, ids := range skipped {
		if len(ids) == 0 {
			continue
		}
		if counts[uid] > 0 {
			next := now.Add(s.batchCooldown())
			if err := s.Store.DeferSubscriptionsFetch(ctx, ids, next.Unix(), s.deferredSubSpacing()); err != nil {
				log.Printf("fetch: user %d defer overflow subscriptions: %v", uid, err)
				continue
			}
			log.Printf("fetch: user %d batch limit reached, deferred %d due subscription(s) until %s",
				uid, len(ids), next.Format(time.RFC3339))
		}
	}
	return out
}

func (s *Scheduler) batchCooldown() time.Duration {
	min, max := s.BatchCooldownMin, s.BatchCooldownMax
	if min <= 0 {
		min = 15 * time.Minute
	}
	if max < min {
		max = min
	}
	if max == min {
		return min
	}
	return min + time.Duration(rand.Int63n(int64(max-min)))
}

func (s *Scheduler) rateLimitBackoff() time.Duration {
	min, max := s.RateLimitBackoffMin, s.RateLimitBackoffMax
	if min <= 0 {
		min = 2 * time.Hour
	}
	if max < min {
		max = min
	}
	if max == min {
		return min
	}
	return min + time.Duration(rand.Int63n(int64(max-min)))
}

func (s *Scheduler) deferredSubSpacing() time.Duration {
	if s.DeferredSubSpacing <= 0 {
		return 10 * time.Minute
	}
	return s.DeferredSubSpacing
}

func (s *Scheduler) deferDueForUser(ctx context.Context, userID int64, delay time.Duration, reason string, rotateLastID int64) {
	if delay <= 0 {
		delay = accounts.AuthRefreshRetryDelay
	}
	now := time.Now()
	first := now.Add(delay)
	n, err := s.Store.DeferDueSubscriptionsByUserRotating(ctx, userID, now.Unix(), first.Unix(), s.deferredSubSpacing(), rotateLastID)
	if err != nil {
		log.Printf("fetch: user %d defer due subscriptions after %s: %v", userID, reason, err)
		return
	}
	if n > 0 {
		if rotateLastID > 0 {
			log.Printf("fetch: user %d deferred %d due subscription(s) after %s until %s; rotated sub %d to end",
				userID, n, reason, first.Format(time.RFC3339), rotateLastID)
			return
		}
		log.Printf("fetch: user %d deferred %d due subscription(s) after %s until %s",
			userID, n, reason, first.Format(time.RFC3339))
	}
}

func (s *Scheduler) deferAllForUser(ctx context.Context, userID int64, delay time.Duration, reason string, rotateLastID int64) {
	if delay <= 0 {
		delay = accounts.AuthRefreshRetryDelay
	}
	now := time.Now()
	first := now.Add(delay)
	n, err := s.Store.DeferAllEnabledSubscriptionsByUserRotating(ctx, userID, first.Unix(), s.deferredSubSpacing(), rotateLastID)
	if err != nil {
		log.Printf("fetch: user %d defer all subscriptions after %s: %v", userID, reason, err)
		return
	}
	if n > 0 {
		if rotateLastID > 0 {
			log.Printf("fetch: user %d deferred all %d subscription(s) after %s until %s; rotated sub %d to end",
				userID, n, reason, first.Format(time.RFC3339), rotateLastID)
			return
		}
		log.Printf("fetch: user %d deferred all %d subscription(s) after %s until %s",
			userID, n, reason, first.Format(time.RFC3339))
	}
}

func minuteOfDayLocal(now time.Time) int {
	lt := now.In(time.Local)
	return lt.Hour()*60 + lt.Minute()
}

func filterByFetchWindow(subs []*model.Subscription, mod int) []*model.Subscription {
	out := subs[:0]
	for _, sub := range subs {
		if inFetchWindow(sub.FetchWindowStartMin, sub.FetchWindowEndMin, mod) {
			out = append(out, sub)
		}
	}
	return out
}

func inFetchWindow(startMin, endMin int64, mod int) bool {
	if startMin < 0 || endMin < 0 {
		return true
	}
	m := int64(mod)
	if startMin <= endMin {
		return m >= startMin && m <= endMin
	}
	return m >= startMin || m <= endMin
}

func (s *Scheduler) interSubSleep(ctx context.Context) {
	min, max := s.InterSubSleepMin, s.InterSubSleepMax
	if max <= min {
		select {
		case <-ctx.Done():
		case <-time.After(min):
		}
		return
	}
	d := min + time.Duration(rand.Int63n(int64(max-min)))
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
