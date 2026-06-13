package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/cry0404/MyWechatRss/internal/model"
	"github.com/cry0404/MyWechatRss/internal/store"
	"github.com/cry0404/MyWechatRss/internal/upstream"
)

type Caller struct {
	Store    *store.Store
	Upstream *upstream.Client

	refreshGuard refreshDebouncer
}

const minRefreshInterval = 10 * time.Second

const CooldownDuration = 30 * time.Minute

const RateLimitCooldownDuration = 4 * time.Hour

const AuthRefreshRetryDelay = 2 * time.Hour

const staleCredentialThreshold = 40 * time.Minute

const MaxRetry = 3

type refreshResult int

const (
	refreshUnavailable refreshResult = iota
	refreshDebounced
	refreshFailed
	refreshSucceeded
)

type refreshDebouncer struct {
	mu   sync.Mutex
	last map[int64]time.Time
}

func (d *refreshDebouncer) allow(accountID int64, now time.Time) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.last == nil {
		d.last = make(map[int64]time.Time)
	}
	if ts, ok := d.last[accountID]; ok && now.Sub(ts) < minRefreshInterval {
		return false
	}
	d.last[accountID] = now
	return true
}

type CallOptions struct {
	Method   string
	Path     string
	Query    map[string]string
	Body     []byte
	BodyType string

	PreferAccountID int64
}

type CallResult struct {
	RawJSON json.RawMessage
	Account *model.WeReadAccount
}

type werrHeader struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

var ErrNoAccount = errors.New("no available weread account (请先扫码绑定或稍后再试)")

var ErrSearchRateLimited = errors.New("weread search/list rate limited")

var ErrHighRiskDeferred = errors.New("weread high-risk call deferred after credential refresh")

func (cr *Caller) Do(ctx context.Context, userID int64, opt CallOptions) (*CallResult, error) {
	var lastErr error
	preferID := opt.PreferAccountID
	for attempt := 0; attempt < MaxRetry; attempt++ {
		acc, err := cr.pickAccount(ctx, userID, preferID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				if lastErr != nil {
					return nil, fmt.Errorf("%w (last err: %w)", ErrNoAccount, lastErr)
				}
				return nil, ErrNoAccount
			}
			return nil, err
		}
		preferID = 0

		body, err := packBody(opt.Body, opt.BodyType)
		if err != nil {
			return nil, err
		}

		if shouldProactiveRefreshBeforeCall(opt.Path, acc) {
			refresh := cr.tryRefresh(ctx, acc, opt.Path)
			if refresh == refreshSucceeded {
				log.Printf("[caller] defer-high-risk-after-proactive-refresh account=%d vid=%d path=%s delay=%s",
					acc.ID, acc.VID, opt.Path, AuthRefreshRetryDelay)
				return nil, fmt.Errorf("%w: account %d refreshed before %s", ErrHighRiskDeferred, acc.ID, opt.Path)
			}
		}
		if shouldWarmupBusinessSessionBeforeCall(opt.Path, acc) {
			cr.warmupBusinessSession(ctx, acc)
		}
		resp, err := cr.Upstream.Call(ctx, upstream.CallReq{
			Credential: upstream.CredentialLite{
				VID:      acc.VID,
				SKey:     acc.SKey,
				Cookies:  acc.Cookies,
				DeviceID: acc.DeviceID,
			},
			Method:   opt.Method,
			Path:     opt.Path,
			Query:    opt.Query,
			Body:     body,
			BodyType: opt.BodyType,
		})
		if err != nil {
			return nil, err
		}

		var hdr werrHeader
		_ = json.Unmarshal(resp.Body, &hdr)

		if resp.Status == 401 || hdr.ErrCode == -2012 {
			signal := fmt.Sprintf("errcode=%d %s", hdr.ErrCode, hdr.ErrMsg)
			if resp.Status == 401 {
				signal = fmt.Sprintf("HTTP 401 on %s", opt.Path)
			}
			log.Printf("[caller] session-expired account=%d vid=%d path=%s signal=%q attempt=%d -> refresh",
				acc.ID, acc.VID, opt.Path, signal, attempt)
			refresh := cr.tryRefresh(ctx, acc, opt.Path)
			if refresh == refreshSucceeded {
				if shouldDeferBusinessRetryAfterRefresh(opt.Path) {
					log.Printf("[caller] defer-retry-after-refresh account=%d vid=%d path=%s delay=%s reason=%q",
						acc.ID, acc.VID, opt.Path, AuthRefreshRetryDelay, signal)
					return nil, fmt.Errorf("%w: account %d refreshed after %s", ErrHighRiskDeferred, acc.ID, signal)
				}
				preferID = acc.ID
				continue
			}
			if refresh == refreshDebounced && attempt+1 < MaxRetry {
				// 另一个 goroutine 刚续期过时，当前请求可能拿的是旧 skey。
				// 重新从 DB pick 一次账号，让它有机会使用刚写入的新凭证。
				log.Printf("[caller] retry-after-recent-refresh account=%d vid=%d path=%s attempt=%d",
					acc.ID, acc.VID, opt.Path, attempt)
				preferID = acc.ID
				lastErr = fmt.Errorf("account %d auth refresh recently attempted: %s", acc.ID, signal)
				continue
			}
			if shouldMarkDeadAfterAuthFailure(opt.Path) {
				log.Printf("[caller] mark-dead account=%d vid=%d reason=%q attempt=%d",
					acc.ID, acc.VID, signal, attempt)
				_ = cr.Store.MarkAccountDead(ctx, acc.UserID, acc.ID, signal)
				lastErr = fmt.Errorf("account %d dead: %s", acc.ID, signal)
				continue
			}
			log.Printf("[caller] cooldown account=%d vid=%d path=%s reason=%q attempt=%d",
				acc.ID, acc.VID, opt.Path, signal, attempt)
			_ = cr.Store.MarkAccountCooldown(ctx, acc.ID, signal, CooldownDuration)
			lastErr = fmt.Errorf("account %d cooldown: %s", acc.ID, signal)
			continue
		}

		switch hdr.ErrCode {
		case 0:
			_ = cr.Store.MarkAccountOK(ctx, acc.ID)
			cr.mergeCookies(ctx, acc, resp.Cookies)
			return &CallResult{RawJSON: resp.Body, Account: acc}, nil

		case -2010:
			log.Printf("[caller] -2010 cooldown account=%d vid=%d path=%s errmsg=%q",
				acc.ID, acc.VID, opt.Path, hdr.ErrMsg)
			_ = cr.Store.MarkAccountCooldown(ctx, acc.ID, "errcode=-2010 "+hdr.ErrMsg, CooldownDuration)
			lastErr = fmt.Errorf("account %d cooldown: -2010 %s", acc.ID, hdr.ErrMsg)
			continue

		case -2041:
			// 搜索/列表接口 (/store/search, /book/articles 等) 的频率风控。
			//
			// 表现：skey/vid 还活着，其他业务 API 正常，只有搜索类路径返 -2041，
			// 同时带 `errlog: CAPw0V0` 之类的 traceId。跟 -2010 的"账号级可疑"不是一档事。
			//
			// 策略：
			//  1. 不 refresh：refresh 不能证明能解除频控，反而增加登录链路请求。
			//  2. 不立即重试：同一窗口连续打只会延长风控。
			//  3. cooldown 而不是 dead：让账号自动恢复，避免不必要的重新扫码。
			log.Printf("[caller] -2041 search-rate-limit account=%d vid=%d path=%s errmsg=%q attempt=%d -> cooldown",
				acc.ID, acc.VID, opt.Path, hdr.ErrMsg, attempt)
			reason := "errcode=-2041 " + hdr.ErrMsg
			_ = cr.Store.MarkAccountCooldown(ctx, acc.ID, reason, RateLimitCooldownDuration)
			lastErr = fmt.Errorf("%w: account %d path=%s: %s", ErrSearchRateLimited, acc.ID, opt.Path, hdr.ErrMsg)
			continue

		default:
			_ = cr.Store.MarkAccountOK(ctx, acc.ID)
			cr.mergeCookies(ctx, acc, resp.Cookies)
			return &CallResult{RawJSON: resp.Body, Account: acc}, nil
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrNoAccount
}

func (cr *Caller) mergeCookies(ctx context.Context, acc *model.WeReadAccount, fresh map[string]string) {
	if len(fresh) == 0 {
		return
	}
	if acc.Cookies == nil {
		acc.Cookies = make(map[string]string, len(fresh))
	}
	changed := false
	for k, v := range fresh {
		if acc.Cookies[k] != v {
			acc.Cookies[k] = v
			changed = true
		}
	}
	if !changed {
		return
	}
	if err := cr.Store.UpdateAccountCookies(ctx, acc.ID, acc.Cookies); err != nil {
		fmt.Printf("[caller] merge cookies for account %d failed: %v\n", acc.ID, err)
	}
}

func (cr *Caller) warmupBusinessSession(ctx context.Context, acc *model.WeReadAccount) {
	cr.warmupMobileSync(ctx, acc)

	resp, err := cr.Upstream.Call(ctx, upstream.CallReq{
		Credential: upstream.CredentialLite{
			VID:      acc.VID,
			SKey:     acc.SKey,
			Cookies:  acc.Cookies,
			DeviceID: acc.DeviceID,
		},
		Method: "GET",
		Path:   "/shelf/sync",
		Query: map[string]string{
			"album":          "1",
			"localBookCount": "0",
			"onlyBookid":     "1",
			"synckey":        "0",
		},
	})
	if err != nil {
		log.Printf("[caller warmup] shelf-sync upstream-error account=%d vid=%d err=%v", acc.ID, acc.VID, err)
		return
	}

	var hdr werrHeader
	_ = json.Unmarshal(resp.Body, &hdr)
	if resp.Status == 200 && hdr.ErrCode == 0 {
		_ = cr.Store.MarkAccountOK(ctx, acc.ID)
		cr.mergeCookies(ctx, acc, resp.Cookies)
		log.Printf("[caller warmup] ok account=%d vid=%d path=/shelf/sync", acc.ID, acc.VID)
		return
	}
	log.Printf("[caller warmup] shelf-sync failed account=%d vid=%d status=%d errcode=%d errmsg=%q",
		acc.ID, acc.VID, resp.Status, hdr.ErrCode, hdr.ErrMsg)
}

func (cr *Caller) warmupMobileSync(ctx context.Context, acc *model.WeReadAccount) {
	body, err := json.Marshal(map[string]any{
		"follower":              0,
		"discoverColumnSynckey": 0,
		"discoverFeedSynckey":   0,
		"reviewTimeline":        0,
		"notifications":         0,
		"inBackground":          0,
		"localBrowseTab":        0,
		"applyList":             0,
		"refluxSynckey":         0,
		"rateSynckey":           0,
		"medal":                 0,
		"chat":                  0,
		"shelf":                 0,
		"localCommunityTab":     0,
		"configsets":            0,
		"medalUrl":              0,
		"booklist":              0,
		"friendReviewSynckey":   0,
		"preferTab":             0,
		"mcard":                 0,
		"following":             0,
		"chatRemovedSynckey":    0,
		"searchSynckey":         0,
		"readingExchange":       0,
		"wehearSyncKey":         0,
		"wechatFriend":          0,
		"config":                0,
		"gift":                  0,
	})
	if err != nil {
		log.Printf("[caller warmup] mobileSync build-body-error account=%d vid=%d err=%v", acc.ID, acc.VID, err)
		return
	}

	resp, err := cr.Upstream.Call(ctx, upstream.CallReq{
		Credential: upstream.CredentialLite{
			VID:      acc.VID,
			SKey:     acc.SKey,
			Cookies:  acc.Cookies,
			DeviceID: acc.DeviceID,
		},
		Method:   "POST",
		Path:     "/mobileSync",
		Body:     json.RawMessage(body),
		BodyType: "json",
	})
	if err != nil {
		log.Printf("[caller warmup] mobileSync upstream-error account=%d vid=%d err=%v", acc.ID, acc.VID, err)
		return
	}

	var hdr werrHeader
	_ = json.Unmarshal(resp.Body, &hdr)
	if resp.Status == 200 && hdr.ErrCode == 0 {
		cr.mergeCookies(ctx, acc, resp.Cookies)
		log.Printf("[caller warmup] ok account=%d vid=%d path=/mobileSync", acc.ID, acc.VID)
		return
	}
	log.Printf("[caller warmup] mobileSync failed account=%d vid=%d status=%d errcode=%d errmsg=%q",
		acc.ID, acc.VID, resp.Status, hdr.ErrCode, hdr.ErrMsg)
}

// ProactiveRefresh 主动对账号做 refreshToken 续期，不依赖 API 错误触发。
// 适合在保活调度器等场景里"提前续期"，避免在业务调用路径上才被动处理 -2012。
// 返回 true 表示续期成功或无需续期（无 refreshToken），false 表示续期失败。
func (cr *Caller) ProactiveRefresh(ctx context.Context, acc *model.WeReadAccount) bool {
	if acc.RefreshToken == "" {
		return true // 没有 refreshToken 就无法主动续期，也不算失败
	}
	if !cr.refreshGuard.allow(acc.ID, time.Now()) {
		return true // 被防抖了，当做"成功"（近期已经续过）
	}
	return cr.doRefresh(ctx, acc, "", "proactive")
}

func (cr *Caller) tryRefresh(ctx context.Context, acc *model.WeReadAccount, refCgi string) refreshResult {
	if acc.RefreshToken == "" {
		log.Printf("[caller refresh] skip account=%d vid=%d refCgi=%q reason=no-refresh-token",
			acc.ID, acc.VID, refCgi)
		return refreshUnavailable
	}
	if !cr.refreshGuard.allow(acc.ID, time.Now()) {
		log.Printf("[caller refresh] skip account=%d vid=%d refCgi=%q reason=debounced (last refresh < %s ago)",
			acc.ID, acc.VID, refCgi, minRefreshInterval)
		return refreshDebounced
	}
	if cr.doRefresh(ctx, acc, refCgi, "on-error") {
		return refreshSucceeded
	}
	return refreshFailed
}

// doRefresh 执行实际的 refreshToken 续期逻辑。
// trigger 参数用于日志标记，如 "on-error"（API 失败后触发）或 "proactive"（保活主动触发）。
func (cr *Caller) doRefresh(ctx context.Context, acc *model.WeReadAccount, refCgi, trigger string) bool {
	log.Printf("[caller refresh] start account=%d vid=%d refCgi=%q trigger=%s",
		acc.ID, acc.VID, refCgi, trigger)
	startAt := time.Now()

	newCred, err := cr.Upstream.LoginRefresh(ctx, upstream.LoginRefreshReq{
		RefreshToken: acc.RefreshToken,
		DeviceID:     acc.DeviceID,
		DeviceName:   acc.DeviceName,
		RefCgi:       refCgi,
	})
	if err != nil {
		log.Printf("[caller refresh] upstream-error account=%d vid=%d err=%v elapsed=%s",
			acc.ID, acc.VID, err, time.Since(startAt))
		return false
	}
	if newCred == nil || newCred.Credential == nil {
		log.Printf("[caller refresh] bad-response account=%d vid=%d reason=missing-credential elapsed=%s",
			acc.ID, acc.VID, time.Since(startAt))
		return false
	}
	cred := newCred.Credential
	if cred.SKey == "" {
		log.Printf("[caller refresh] bad-response account=%d vid=%d reason=empty-skey elapsed=%s",
			acc.ID, acc.VID, time.Since(startAt))
		return false
	}

	// Benign refresh 时 weread 可能只回 skey，refreshToken / cookies 留空。
	// 用 nil 指针显式表示"这次别动这列"，防止把还能用的旧值抹成空串。
	var rtArg *string
	if cred.RefreshToken != "" {
		rtArg = &cred.RefreshToken
	}
	var ckArg *map[string]string
	if len(cred.Cookies) > 0 {
		ckArg = &cred.Cookies
	}

	if err := cr.Store.UpdateAccountCredential(ctx, acc.ID, cred.SKey, rtArg, ckArg); err != nil {
		log.Printf("[caller refresh] save-error account=%d vid=%d err=%v elapsed=%s",
			acc.ID, acc.VID, err, time.Since(startAt))
		return false
	}

	oldRT := acc.RefreshToken
	acc.SKey = cred.SKey
	if rtArg != nil {
		acc.RefreshToken = cred.RefreshToken
	}
	if ckArg != nil {
		acc.Cookies = cred.Cookies
	}

	log.Printf("[caller refresh] ok account=%d vid=%d newVid=%d skeyLen=%d rtRolled=%t ckRolled=%t trigger=%s elapsed=%s",
		acc.ID, acc.VID, cred.VID, len(cred.SKey),
		rtArg != nil && cred.RefreshToken != oldRT,
		ckArg != nil,
		trigger,
		time.Since(startAt))
	return true
}

func (cr *Caller) pickAccount(ctx context.Context, userID, preferID int64) (*model.WeReadAccount, error) {
	if preferID > 0 {
		acc, err := cr.Store.GetActiveAccountByID(ctx, userID, preferID)
		if err == nil {
			return acc, nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
	}
	return cr.Store.PickActiveAccount(ctx, userID)
}

func shouldMarkDeadAfterAuthFailure(path string) bool {
	switch path {
	case "/device/sessionlist":
		return true
	default:
		return false
	}
}

func shouldProactiveRefreshBeforeCall(path string, acc *model.WeReadAccount) bool {
	if !shouldDeferBusinessRetryAfterRefresh(path) {
		return false
	}
	if acc.RefreshToken == "" || acc.LastOkAt == 0 {
		return false
	}
	return time.Since(time.Unix(acc.LastOkAt, 0)) > staleCredentialThreshold
}

func shouldWarmupBusinessSessionBeforeCall(path string, acc *model.WeReadAccount) bool {
	return shouldDeferBusinessRetryAfterRefresh(path) && strings.HasPrefix(acc.LastErr, "errcode=-2041")
}

func shouldDeferBusinessRetryAfterRefresh(path string) bool {
	switch path {
	case "/book/articles",
		"/store/search",
		"/book/info",
		"/book/getProgress",
		"/book/readinfo",
		"/book/detailinfo",
		"/review/single",
		"/book/shareChapter":
		return true
	default:
		return false
	}
}

func packBody(body []byte, bodyType string) (json.RawMessage, error) {
	if len(body) == 0 {
		return nil, nil
	}
	if bodyType == "form" {
		return json.Marshal(string(body))
	}
	return json.RawMessage(body), nil
}
