package keepalive

import (
	"context"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/cry0404/MyWechatRss/internal/accounts"
	"github.com/cry0404/MyWechatRss/internal/store"
)

// staleThreshold 定义账号多久没活动就算 "stale"，需要主动续期。
// 微信读书 skey 寿命约 1.5h，Android 客户端在后台 >40min 后回前台会刷新。
// 这里取 40 分钟，比官方稍保守。
const staleThreshold = 40 * time.Minute

// Scheduler 定期对已绑定的 weread 账号做轻量心跳，并在需要时主动续期 skey。
type Scheduler struct {
	Store  *store.Store
	Caller *accounts.Caller

	// MinInterval / MaxInterval 控制两次调度之间的随机间隔。
	MinInterval time.Duration
	MaxInterval time.Duration

	// InterAccountSleep 是连续调用两个账号之间的随机休眠范围。
	InterAccountSleepMin time.Duration
	InterAccountSleepMax time.Duration

	// ConsecutiveFailThreshold 连续失败多少次后打 warning 日志。
	ConsecutiveFailThreshold int

	failCount sync.Map // map[int64]int 账号 ID -> 连续失败次数
}

func NewScheduler(st *store.Store, caller *accounts.Caller) *Scheduler {
	return &Scheduler{
		Store:                    st,
		Caller:                   caller,
		MinInterval:              35 * time.Minute,
		MaxInterval:              55 * time.Minute,
		InterAccountSleepMin:     5 * time.Second,
		InterAccountSleepMax:     30 * time.Second,
		ConsecutiveFailThreshold: 3,
	}
}

func (s *Scheduler) Run(ctx context.Context) {
	if s.MinInterval <= 0 {
		s.MinInterval = 35 * time.Minute
	}
	if s.MaxInterval < s.MinInterval {
		s.MaxInterval = s.MinInterval
	}

	log.Printf("keepalive scheduler started, interval=%s~%s, staleThreshold=%s",
		s.MinInterval, s.MaxInterval, staleThreshold)

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

	now := time.Now()
	var staleCount, okCount, failCount int

	log.Printf("keepalive: checking %d active account(s)", len(accs))

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

		// 判断账号是否 stale：last_ok_at 为空或超过阈值未更新。
		lastOk := time.Unix(acc.LastOkAt, 0)
		isStale := acc.LastOkAt == 0 || now.Sub(lastOk) > staleThreshold

		if isStale {
			// 主动续期：避免在业务调用时才被动处理 -2012。
			refreshed := s.Caller.ProactiveRefresh(ctx, acc)
			if !refreshed {
				s.recordFail(acc.ID)
				log.Printf("keepalive: account=%d vid=%d proactive-refresh FAILED (consecutive=%d)",
					acc.ID, acc.VID, s.getFailCount(acc.ID))
				staleCount++
				continue
			}
			log.Printf("keepalive: account=%d vid=%d proactive-refresh ok", acc.ID, acc.VID)
		}

		// 用轻量 API 做心跳验证：/device/sessionlist 比 /shelf/sync 更轻。
		_, err := s.Caller.Do(ctx, acc.UserID, accounts.CallOptions{
			Method: http.MethodGet,
			Path:   "/device/sessionlist",
			Query: map[string]string{
				"deviceId": acc.DeviceID,
				"onlyCnt":  "1",
			},
			PreferAccountID: acc.ID,
		})
		if err != nil {
			s.recordFail(acc.ID)
			fc := s.getFailCount(acc.ID)
			log.Printf("keepalive: account=%d vid=%d heartbeat FAILED (consecutive=%d): %v",
				acc.ID, acc.VID, fc, err)
			if fc >= s.ConsecutiveFailThreshold {
				log.Printf("keepalive: WARNING account=%d vid=%d has failed %d consecutive checks, may need rescan",
					acc.ID, acc.VID, fc)
			}
			failCount++
			continue
		}

		s.resetFail(acc.ID)
		okCount++
		log.Printf("keepalive: account=%d vid=%d heartbeat ok (stale=%t)", acc.ID, acc.VID, isStale)
	}

	log.Printf("keepalive: round complete stale=%d ok=%d fail=%d", staleCount, okCount, failCount)
}

func (s *Scheduler) recordFail(id int64) {
	v, _ := s.failCount.LoadOrStore(id, 0)
	s.failCount.Store(id, v.(int)+1)
}

func (s *Scheduler) resetFail(id int64) {
	s.failCount.Delete(id)
}

func (s *Scheduler) getFailCount(id int64) int {
	v, ok := s.failCount.Load(id)
	if !ok {
		return 0
	}
	return v.(int)
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
