package articles

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cry0404/MyWechatRss/internal/accounts"
	"github.com/cry0404/MyWechatRss/internal/crypto"
	"github.com/cry0404/MyWechatRss/internal/model"
	"github.com/cry0404/MyWechatRss/internal/store"
	"github.com/cry0404/MyWechatRss/internal/upstream"
)

func TestSchedulerPersistsOverflowDeferral(t *testing.T) {
	ctx := context.Background()
	st, user := newSchedulerTestStore(t, 5)
	var articleCalls int32

	up := articleTestUpstreamClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxy/weread/call" {
			http.NotFound(w, r)
			return
		}
		var req upstream.CallReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode call req: %v", err)
		}
		if req.Path == "/book/articles" && req.Query["offset"] == "0" {
			atomic.AddInt32(&articleCalls, 1)
		}
		writeArticleJSON(t, w, upstream.CallResp{
			Status:  http.StatusOK,
			Headers: map[string]string{},
			Body:    json.RawMessage(`{"errcode":0,"reviews":[]}`),
		})
	})

	svc := &Service{
		Store:  st,
		Caller: &accounts.Caller{Store: st, Upstream: up},
		Mode:   "summary",
	}
	scheduler := NewScheduler(st, svc)
	scheduler.InterSubSleepMin = 0
	scheduler.InterSubSleepMax = 0
	scheduler.MaxSubsPerBatch = 2
	scheduler.BatchCooldownMin = time.Hour
	scheduler.BatchCooldownMax = time.Hour
	scheduler.DeferredSubSpacing = 10 * time.Minute

	scheduler.runOnce(ctx)
	if got := atomic.LoadInt32(&articleCalls); got != 2 {
		t.Fatalf("first batch fetched %d subscriptions, want 2", got)
	}

	scheduler.runOnce(ctx)
	if got := atomic.LoadInt32(&articleCalls); got != 2 {
		t.Fatalf("second immediate batch fetched more subscriptions, total=%d want 2", got)
	}

	due, err := st.ListSubscriptionsDueForFetch(ctx, time.Now().Unix())
	if err != nil {
		t.Fatalf("ListSubscriptionsDueForFetch: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("remaining due subscriptions=%d want 0 for user %d", len(due), user.ID)
	}

	rows, err := st.DB().QueryContext(ctx, `
		SELECT next_fetch_after FROM subscriptions
		WHERE user_id = ? AND last_fetch_at = 0
		ORDER BY next_fetch_after ASC
	`, user.ID)
	if err != nil {
		t.Fatalf("query deferred subscriptions: %v", err)
	}
	defer rows.Close()
	var deferred []int64
	for rows.Next() {
		var ts int64
		if err := rows.Scan(&ts); err != nil {
			t.Fatalf("scan next_fetch_after: %v", err)
		}
		deferred = append(deferred, ts)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	if len(deferred) != 3 {
		t.Fatalf("deferred subscriptions=%d want 3", len(deferred))
	}
	if !(deferred[0] < deferred[1] && deferred[1] < deferred[2]) {
		t.Fatalf("deferred subscriptions should be staggered, got %v", deferred)
	}
}

func TestSchedulerDefersDueBacklogAfterRateLimit(t *testing.T) {
	ctx := context.Background()
	st, user := newSchedulerTestStore(t, 5)
	now := time.Now()
	var futureID int64
	if err := st.DB().QueryRowContext(ctx, `
		SELECT id FROM subscriptions WHERE user_id = ? ORDER BY id DESC LIMIT 1
	`, user.ID).Scan(&futureID); err != nil {
		t.Fatalf("query future subscription: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx, `
		UPDATE subscriptions SET next_fetch_after = ? WHERE id = ?
	`, now.Add(30*time.Minute).Unix(), futureID); err != nil {
		t.Fatalf("set future subscription: %v", err)
	}

	var articleCalls int32
	var attemptedBookID string

	up := articleTestUpstreamClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxy/weread/call" {
			http.NotFound(w, r)
			return
		}
		var req upstream.CallReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode call req: %v", err)
		}
		if req.Path == "/book/articles" {
			atomic.AddInt32(&articleCalls, 1)
			attemptedBookID = req.Query["bookId"]
			writeArticleJSON(t, w, upstream.CallResp{
				Status:  http.StatusOK,
				Headers: map[string]string{},
				Body:    json.RawMessage(`{"errcode":-2041,"errmsg":"-2041"}`),
			})
			return
		}
		writeArticleJSON(t, w, upstream.CallResp{
			Status:  http.StatusOK,
			Headers: map[string]string{},
			Body:    json.RawMessage(`{"errcode":0}`),
		})
	})

	svc := &Service{
		Store:  st,
		Caller: &accounts.Caller{Store: st, Upstream: up},
		Mode:   "summary",
	}
	scheduler := NewScheduler(st, svc)
	scheduler.InterSubSleepMin = 0
	scheduler.InterSubSleepMax = 0
	scheduler.MaxSubsPerBatch = 2
	scheduler.BatchCooldownMin = time.Hour
	scheduler.BatchCooldownMax = time.Hour
	scheduler.DeferredSubSpacing = 10 * time.Minute
	scheduler.RateLimitBackoffMin = 2 * time.Hour
	scheduler.RateLimitBackoffMax = 2 * time.Hour

	scheduler.runOnce(ctx)
	if got := atomic.LoadInt32(&articleCalls); got != 1 {
		t.Fatalf("rate limit should stop user batch after first call, got %d calls", got)
	}

	due, err := st.ListSubscriptionsDueForFetch(ctx, time.Now().Unix())
	if err != nil {
		t.Fatalf("ListSubscriptionsDueForFetch: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("due subscriptions after rate limit=%d want 0", len(due))
	}
	accs, err := st.ListAccountsByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListAccountsByUser: %v", err)
	}
	if len(accs) != 1 || accs[0].Status != model.AccountCooldown {
		t.Fatalf("expected one cooldown account, got %#v", accs)
	}

	var attemptedID int64
	if err := st.DB().QueryRowContext(ctx, `
		SELECT id FROM subscriptions WHERE user_id = ? AND book_id = ?
	`, user.ID, attemptedBookID).Scan(&attemptedID); err != nil {
		t.Fatalf("query attempted subscription: %v", err)
	}
	rows, err := st.DB().QueryContext(ctx, `
		SELECT id FROM subscriptions
		WHERE user_id = ?
		ORDER BY next_fetch_after ASC, id ASC
	`, user.ID)
	if err != nil {
		t.Fatalf("query deferred order: %v", err)
	}
	defer rows.Close()
	var ordered []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan deferred order: %v", err)
		}
		ordered = append(ordered, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("deferred order rows: %v", err)
	}
	if len(ordered) == 0 {
		t.Fatal("expected deferred subscriptions")
	}
	if ordered[0] == attemptedID {
		t.Fatalf("rate-limit probe should rotate failed subscription away from the next probe, order=%v attempted=%d", ordered, attemptedID)
	}
	if ordered[len(ordered)-1] != attemptedID {
		t.Fatalf("rate-limit probe should put failed subscription last, order=%v attempted=%d", ordered, attemptedID)
	}

	minNext := now.Add(2*time.Hour - time.Minute).Unix()
	var tooEarly int
	if err := st.DB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM subscriptions
		WHERE user_id = ?
		  AND disabled = 0
		  AND next_fetch_after < ?
	`, user.ID, minNext).Scan(&tooEarly); err != nil {
		t.Fatalf("query too-early deferrals: %v", err)
	}
	if tooEarly != 0 {
		t.Fatalf("rate limit should defer all enabled subscriptions past recovery window, tooEarly=%d", tooEarly)
	}
}

func TestSchedulerRunsSingleProbeWhileRecoveringFromRateLimit(t *testing.T) {
	ctx := context.Background()
	st, user := newSchedulerTestStore(t, 5)
	if _, err := st.DB().ExecContext(ctx, `
		UPDATE weread_accounts SET last_err = 'errcode=-2041 -2041' WHERE user_id = ?
	`, user.ID); err != nil {
		t.Fatalf("set recovery state: %v", err)
	}
	var articleCalls int32

	up := articleTestUpstreamClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxy/weread/call" {
			http.NotFound(w, r)
			return
		}
		var req upstream.CallReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode call req: %v", err)
		}
		if req.Path == "/book/articles" {
			atomic.AddInt32(&articleCalls, 1)
		}
		writeArticleJSON(t, w, upstream.CallResp{
			Status:  http.StatusOK,
			Headers: map[string]string{},
			Body:    json.RawMessage(`{"errcode":0,"reviews":[]}`),
		})
	})

	svc := &Service{
		Store:  st,
		Caller: &accounts.Caller{Store: st, Upstream: up},
		Mode:   "summary",
	}
	scheduler := NewScheduler(st, svc)
	scheduler.InterSubSleepMin = 0
	scheduler.InterSubSleepMax = 0
	scheduler.BatchCooldownMin = time.Hour
	scheduler.BatchCooldownMax = time.Hour
	scheduler.DeferredSubSpacing = 10 * time.Minute

	scheduler.runOnce(ctx)
	if got := atomic.LoadInt32(&articleCalls); got != 1 {
		t.Fatalf("recovery should probe one subscription, got %d article calls", got)
	}

	due, err := st.ListSubscriptionsDueForFetch(ctx, time.Now().Unix())
	if err != nil {
		t.Fatalf("ListSubscriptionsDueForFetch: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("recovery overflow should be deferred, due=%d want 0", len(due))
	}
}

func TestFetchAllStopsAfterRateLimit(t *testing.T) {
	ctx := context.Background()
	st, user := newSchedulerTestStore(t, 3)
	var calls int32
	svc := &Service{
		Store: st,
		Caller: &accounts.Caller{
			Store: st,
			Upstream: articleTestUpstreamClient(t, func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&calls, 1)
				writeArticleJSON(t, w, upstream.CallResp{
					Status:  http.StatusOK,
					Headers: map[string]string{},
					Body:    json.RawMessage(`{"errcode":-2041,"errmsg":"-2041"}`),
				})
			}),
		},
		Mode: "summary",
	}

	_, err := svc.FetchAll(ctx, user.ID)
	if !errors.Is(err, accounts.ErrSearchRateLimited) {
		t.Fatalf("expected ErrSearchRateLimited, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("FetchAll should stop after rate limit, got %d calls", got)
	}
}

func newSchedulerTestStore(t *testing.T, subCount int) (*store.Store, *model.User) {
	t.Helper()
	codec, err := crypto.New("test-secret-with-enough-length")
	if err != nil {
		t.Fatalf("crypto.New: %v", err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"), codec)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	ctx := context.Background()
	user := &model.User{Username: "u", Email: "u@example.com", PasswordHash: "hash"}
	if err := st.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	acc := &model.WeReadAccount{
		UserID:       user.ID,
		VID:          565310662,
		SKey:         "skey",
		RefreshToken: "refresh-token",
		Cookies:      map[string]string{},
		Status:       model.AccountActive,
		DeviceID:     "dev",
		InstallID:    "install",
		DeviceName:   "device",
	}
	if err := st.CreateAccount(ctx, acc); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	for i := 0; i < subCount; i++ {
		sub := &model.Subscription{
			UserID: user.ID,
			BookID: "MP_WXS_sched_" + string(rune('A'+i)),
			Alias:  "sub",
		}
		if err := st.CreateSubscription(ctx, sub); err != nil {
			t.Fatalf("CreateSubscription %d: %v", i, err)
		}
	}
	return st, user
}
