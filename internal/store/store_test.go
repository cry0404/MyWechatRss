package store_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/cry0404/MyWechatRss/internal/crypto"
	"github.com/cry0404/MyWechatRss/internal/model"
	"github.com/cry0404/MyWechatRss/internal/store"
)

func TestOpenClampsExistingRateLimitCooldowns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	codec, err := crypto.New("test-secret-with-enough-length")
	if err != nil {
		t.Fatalf("crypto.New: %v", err)
	}

	st, err := store.Open(dbPath, codec)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	ctx := context.Background()
	user := &model.User{Username: "u", Email: "u@example.com", PasswordHash: "hash"}
	if err := st.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	acc := &model.WeReadAccount{
		UserID:  user.ID,
		VID:     565310662,
		SKey:    "skey",
		Cookies: map[string]string{"wr_vid": "565310662"},
		Status:  model.AccountActive,
	}
	if err := st.CreateAccount(ctx, acc); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	tooLongUntil := time.Now().Add(4 * time.Hour).Unix()
	if _, err := st.DB().ExecContext(ctx, `
		UPDATE weread_accounts
		SET status = 'cooldown', cooldown_until = ?, last_err = 'errcode=-2041 -2041'
		WHERE id = ?
	`, tooLongUntil, acc.ID); err != nil {
		t.Fatalf("set cooldown: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	st, err = store.Open(dbPath, codec)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer st.Close()

	var got int64
	if err := st.DB().QueryRowContext(ctx,
		`SELECT cooldown_until FROM weread_accounts WHERE id = ?`, acc.ID,
	).Scan(&got); err != nil {
		t.Fatalf("query cooldown: %v", err)
	}
	maxUntil := time.Now().Add(50 * time.Minute).Unix()
	if got > maxUntil {
		t.Fatalf("cooldown_until=%d want <= %d", got, maxUntil)
	}
}

func TestRecordSubscriptionFetchLogAnnotatesRateLimitInterval(t *testing.T) {
	st, user, sub := newStoreTestSubscription(t)
	ctx := context.Background()

	first := &model.SubscriptionFetchLog{
		SubscriptionID: sub.ID,
		AccountID:      11,
		StartedAt:      1_000,
		CostMs:         120,
		Error:          "weread search/list rate limited: errcode=-2041 -2041",
	}
	if err := st.RecordSubscriptionFetchLog(ctx, first); err != nil {
		t.Fatalf("RecordSubscriptionFetchLog first: %v", err)
	}
	if first.ErrorCode != "-2041" {
		t.Fatalf("first error_code=%q want -2041", first.ErrorCode)
	}
	if first.PreviousRateLimitAt != 0 || first.SecondsSinceLastRateLimit != 0 {
		t.Fatalf("first interval=(%d,%d), want no previous", first.PreviousRateLimitAt, first.SecondsSinceLastRateLimit)
	}

	second := &model.SubscriptionFetchLog{
		SubscriptionID: sub.ID,
		AccountID:      11,
		StartedAt:      1_900,
		CostMs:         150,
		Error:          "account 11 path=/book/articles: errcode=-2041 -2041",
	}
	if err := st.RecordSubscriptionFetchLog(ctx, second); err != nil {
		t.Fatalf("RecordSubscriptionFetchLog second: %v", err)
	}
	if second.PreviousRateLimitAt != first.StartedAt {
		t.Fatalf("previous_rate_limit_at=%d want %d", second.PreviousRateLimitAt, first.StartedAt)
	}
	if second.SecondsSinceLastRateLimit != 900 {
		t.Fatalf("seconds_since_last_rate_limit=%d want 900", second.SecondsSinceLastRateLimit)
	}

	events, err := st.ListFetchEvents(ctx, user.ID, 20, 0, true)
	if err != nil {
		t.Fatalf("ListFetchEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events len=%d want 2", len(events))
	}
	if events[0].SubscriptionID != sub.ID || events[0].SubscriptionAlias != sub.Alias {
		t.Fatalf("event subscription=(%d,%q), want (%d,%q)", events[0].SubscriptionID, events[0].SubscriptionAlias, sub.ID, sub.Alias)
	}
	if events[0].Chain != "source" || events[0].ErrorCode != "-2041" || events[0].SecondsSinceLastRateLimit != 900 {
		t.Fatalf("event chain/error/interval=(%q,%q,%d), want source/-2041/900", events[0].Chain, events[0].ErrorCode, events[0].SecondsSinceLastRateLimit)
	}
}

func TestListFetchEventsIncludesSourceAndContentChains(t *testing.T) {
	st, user, sub := newStoreTestSubscription(t)
	ctx := context.Background()

	if err := st.RecordSubscriptionFetchLog(ctx, &model.SubscriptionFetchLog{
		SubscriptionID: sub.ID,
		AccountID:      3,
		StartedAt:      2_000,
		CostMs:         80,
		NewCount:       2,
	}); err != nil {
		t.Fatalf("RecordSubscriptionFetchLog: %v", err)
	}
	if err := st.RecordArticleFetchLog(ctx, &model.ArticleFetchLog{
		ReviewID:  "review-1",
		BookID:    sub.BookID,
		Chain:     "web",
		Success:   false,
		CostMs:    40,
		Error:     "#js_content 不存在",
		CreatedAt: 2_010,
	}); err != nil {
		t.Fatalf("RecordArticleFetchLog: %v", err)
	}

	events, err := st.ListFetchEvents(ctx, user.ID, 20, 0, false)
	if err != nil {
		t.Fatalf("ListFetchEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events len=%d want 2", len(events))
	}
	if events[0].Chain != "web" || events[0].ReviewID != "review-1" {
		t.Fatalf("first event=(%q,%q), want web/review-1", events[0].Chain, events[0].ReviewID)
	}
	if events[1].Chain != "source" || events[1].NewCount != 2 {
		t.Fatalf("second event=(%q,%d), want source/2", events[1].Chain, events[1].NewCount)
	}
}

func TestDeferDueSubscriptionsByUserRotatingMovesSelectedSubscriptionLast(t *testing.T) {
	st, user, _ := newStoreTestSubscription(t)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		sub := &model.Subscription{
			UserID: user.ID,
			BookID: "MP_WXS_extra_" + string(rune('A'+i)),
			Alias:  "extra",
		}
		if err := st.CreateSubscription(ctx, sub); err != nil {
			t.Fatalf("CreateSubscription extra %d: %v", i, err)
		}
	}

	now := time.Now().Unix()
	var firstID int64
	if err := st.DB().QueryRowContext(ctx, `
		SELECT id FROM subscriptions
		WHERE user_id = ?
		ORDER BY id ASC
		LIMIT 1
	`, user.ID).Scan(&firstID); err != nil {
		t.Fatalf("query first subscription: %v", err)
	}

	n, err := st.DeferDueSubscriptionsByUserRotating(ctx, user.ID, now, now+3600, 600*time.Second, firstID)
	if err != nil {
		t.Fatalf("DeferDueSubscriptionsByUserRotating: %v", err)
	}
	if n != 3 {
		t.Fatalf("deferred %d subscriptions, want 3", n)
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
		t.Fatalf("rows: %v", err)
	}
	if ordered[len(ordered)-1] != firstID {
		t.Fatalf("rotated order=%v, want first id %d last", ordered, firstID)
	}
}

func newStoreTestSubscription(t *testing.T) (*store.Store, *model.User, *model.Subscription) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	codec, err := crypto.New("test-secret-with-enough-length")
	if err != nil {
		t.Fatalf("crypto.New: %v", err)
	}
	st, err := store.Open(dbPath, codec)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	ctx := context.Background()
	user := &model.User{Username: "u", Email: "u@example.com", PasswordHash: "hash"}
	if err := st.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	sub := &model.Subscription{
		UserID: user.ID,
		BookID: "MP_WXS_1",
		Alias:  "测试订阅",
		MPName: "测试公众号",
	}
	if err := st.CreateSubscription(ctx, sub); err != nil {
		t.Fatalf("CreateSubscription: %v", err)
	}
	return st, user, sub
}
