package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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

const MaxRetry = 3

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

func (cr *Caller) Do(ctx context.Context, userID int64, opt CallOptions) (*CallResult, error) {
	var lastErr error
	preferID := opt.PreferAccountID
	// triedRefreshFor2041 记录"这一次 Do() 调用内，针对当前 account 是否已经
	// 为 -2041 尝试过一次 refresh"。跨 account（failover 后换号）时会被 picker
	// 切换掉的账号自动带过去（保守策略：单账号场景我们其实也不会真换号）。
	triedRefreshFor2041 := false
	for attempt := 0; attempt < MaxRetry; attempt++ {
		acc, err := cr.pickAccount(ctx, userID, preferID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				if lastErr != nil {
					return nil, fmt.Errorf("%w (last err: %v)", ErrNoAccount, lastErr)
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
			if cr.tryRefresh(ctx, acc, opt.Path) {
				preferID = acc.ID
				continue
			}
			log.Printf("[caller] mark-dead account=%d vid=%d reason=%q attempt=%d",
				acc.ID, acc.VID, signal, attempt)
			_ = cr.Store.MarkAccountDead(ctx, acc.UserID, acc.ID, signal)
			lastErr = fmt.Errorf("account %d dead: %s", acc.ID, signal)
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
			// 搜索接口 (/store/search 等) 专属的频率风控。
			//
			// 表现：skey/vid 还活着，其他业务 API 正常，只有搜索类路径返 -2041，
			// 同时带 `errlog: CAPw0V0` 之类的 traceId。跟 -2010 的"账号级可疑"不是一档事。
			//
			// 策略：
			//  1. 第一次碰到 → 尝试 refresh 一次（换 skey 有时能让搜索维度计数重置）；
			//     refresh 成功就 preferID 回原账号重试一次原请求；
			//  2. refresh 起不了作用 / 重试还是 -2041 → 直接返回错误，不 cooldown 账号。
			//     cooldown 会导致单账号场景下服务完全不可用，且频繁触发时会"一直 cooldown
			//     永不恢复"。-2041 本身是频率限制不是账号失效，返回错误让调用方（scheduler）
			//     下轮重试即可。
			//  3. 全程不 mark dead；-2041 本身不是 skey 失效信号。
			log.Printf("[caller] -2041 search-rate-limit account=%d vid=%d path=%s errmsg=%q attempt=%d triedRefresh=%t",
				acc.ID, acc.VID, opt.Path, hdr.ErrMsg, attempt, triedRefreshFor2041)
			if !triedRefreshFor2041 && cr.tryRefresh(ctx, acc, opt.Path) {
				triedRefreshFor2041 = true
				preferID = acc.ID
				continue
			}
			// 不 cooldown：-2041 是频率限制，不是账号失效。cooldown 会让服务在 5 分钟内
			// 完全不可用，且频繁请求时会反复刷新 cooldown 时间导致"永远恢复不了"。
			lastErr = fmt.Errorf("account %d search rate-limited (-2041): %s", acc.ID, hdr.ErrMsg)
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

func (cr *Caller) tryRefresh(ctx context.Context, acc *model.WeReadAccount, refCgi string) bool {
	if acc.RefreshToken == "" {
		log.Printf("[caller refresh] skip account=%d vid=%d refCgi=%q reason=no-refresh-token",
			acc.ID, acc.VID, refCgi)
		return false
	}
	if !cr.refreshGuard.allow(acc.ID, time.Now()) {
		log.Printf("[caller refresh] skip account=%d vid=%d refCgi=%q reason=debounced (last refresh < %s ago)",
			acc.ID, acc.VID, refCgi, minRefreshInterval)
		return false
	}
	return cr.doRefresh(ctx, acc, refCgi, "on-error")
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

func packBody(body []byte, bodyType string) (json.RawMessage, error) {
	if len(body) == 0 {
		return nil, nil
	}
	if bodyType == "form" {
		return json.Marshal(string(body))
	}
	return json.RawMessage(body), nil
}
