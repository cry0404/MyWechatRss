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
	log.Printf("[caller refresh] start account=%d vid=%d refCgi=%q",
		acc.ID, acc.VID, refCgi)
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

	log.Printf("[caller refresh] ok account=%d vid=%d newVid=%d skeyLen=%d rtRolled=%t ckRolled=%t elapsed=%s",
		acc.ID, acc.VID, cred.VID, len(cred.SKey),
		rtArg != nil && cred.RefreshToken != oldRT,
		ckArg != nil,
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
