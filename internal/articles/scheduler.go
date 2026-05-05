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

	warnedUsers sync.Map // map[int64]struct{}
}

func NewScheduler(st *store.Store, svc *Service) *Scheduler {
	return &Scheduler{
		Store:            st,
		Service:          svc,
		Tick:             time.Minute,
		InterSubSleepMin: 30 * time.Second,
		InterSubSleepMax: 120 * time.Second,
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

	mod := minuteOfDayLocal(time.Now())
	runnable = filterByFetchWindow(runnable, mod)

	for i, sub := range runnable {
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
