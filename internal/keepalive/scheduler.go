package keepalive

import (
	"context"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/cry0404/MyWechatRss/internal/accounts"
	"github.com/cry0404/MyWechatRss/internal/store"
)

// Scheduler 定期通过已绑定的 weread 账号调用轻量 API，保持会话活跃。
// 太久不刷新会导致搜索接口 (/store/search) 失效。
type Scheduler struct {
	Store  *store.Store
	Caller *accounts.Caller

	// MinInterval / MaxInterval 控制两次保活之间的随机间隔。
	MinInterval time.Duration
	MaxInterval time.Duration

	// InterAccountSleep 是连续调用两个账号之间的随机休眠范围。
	InterAccountSleepMin time.Duration
	InterAccountSleepMax time.Duration
}

func NewScheduler(st *store.Store, caller *accounts.Caller) *Scheduler {
	return &Scheduler{
		Store:                st,
		Caller:               caller,
		MinInterval:          2 * time.Hour,
		MaxInterval:          6 * time.Hour,
		InterAccountSleepMin: 5 * time.Second,
		InterAccountSleepMax: 30 * time.Second,
	}
}

func (s *Scheduler) Run(ctx context.Context) {
	if s.MinInterval <= 0 {
		s.MinInterval = 2 * time.Hour
	}
	if s.MaxInterval < s.MinInterval {
		s.MaxInterval = s.MinInterval
	}

	log.Printf("keepalive scheduler started, interval=%s~%s", s.MinInterval, s.MaxInterval)

	// 首次执行前先等一个随机间隔，避免所有实例同时启动时集中请求。
	s.sleepRandom(ctx, s.MinInterval, s.MaxInterval)

	for {
		if ctx.Err() != nil {
			return
		}
		s.runOnce(ctx)
		s.sleepRandom(ctx, s.MinInterval, s.MaxInterval)
	}
}

func (s *Scheduler) runOnce(ctx context.Context) {
	accs, err := s.Store.ListAllActiveAccounts(ctx)
	if err != nil {
		log.Printf("keepalive: list active accounts: %v", err)
		return
	}
	if len(accs) == 0 {
		return
	}

	log.Printf("keepalive: pinging %d active account(s)", len(accs))

	for i, acc := range accs {
		if ctx.Err() != nil {
			return
		}
		if i > 0 {
			s.sleepRandom(ctx, s.InterAccountSleepMin, s.InterAccountSleepMax)
			if ctx.Err() != nil {
				return
			}
		}

		// 调用轻量 API：/shelf/sync 是 App 回前台时的标准同步接口，
		// onlyBookid=1 让响应最小化，副作用等同于"刷新状态"。
		_, err := s.Caller.Do(ctx, acc.UserID, accounts.CallOptions{
			Method: http.MethodGet,
			Path:   "/shelf/sync",
			Query: map[string]string{
				"synckey":        "0",
				"onlyBookid":     "1",
				"album":          "1",
				"localBookCount": "0",
			},
			PreferAccountID: acc.ID,
		})
		if err != nil {
			log.Printf("keepalive: account=%d vid=%d err=%v", acc.ID, acc.VID, err)
			continue
		}
		log.Printf("keepalive: account=%d vid=%d ok", acc.ID, acc.VID)
	}
}

func (s *Scheduler) sleepRandom(ctx context.Context, min, max time.Duration) {
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
