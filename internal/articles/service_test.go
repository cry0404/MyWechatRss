package articles

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cry0404/MyWechatRss/internal/accounts"
	"github.com/cry0404/MyWechatRss/internal/crypto"
	"github.com/cry0404/MyWechatRss/internal/model"
	"github.com/cry0404/MyWechatRss/internal/store"
	"github.com/cry0404/MyWechatRss/internal/upstream"
)

func TestFetchLatestReturnsNoAccountSentinelWhenNoActiveAccountExists(t *testing.T) {
	st, user, sub := newArticleTestStore(t)
	svc := &Service{Store: st}

	_, err := svc.FetchLatest(context.Background(), user.ID, sub.ID)
	if !errors.Is(err, accounts.ErrNoAccount) {
		t.Fatalf("expected ErrNoAccount, got %v", err)
	}
}

func TestFetchContentViaShareChapterUsesSingleProtocolAttempt(t *testing.T) {
	ctx := context.Background()
	st, user, _ := newArticleTestStore(t)
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

	var shareChapterCalls int32
	var paths []string
	up := articleTestUpstreamClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxy/weread/call" {
			http.NotFound(w, r)
			return
		}
		var req upstream.CallReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode call req: %v", err)
		}
		paths = append(paths, req.Path)
		if req.Path == "/book/shareChapter" {
			atomic.AddInt32(&shareChapterCalls, 1)
		}
		writeArticleJSON(t, w, upstream.CallResp{
			Status:  http.StatusOK,
			Headers: map[string]string{},
			Body:    json.RawMessage(`{"errcode":0,"data":{}}`),
		})
	})

	svc := &Service{
		Store:  st,
		Caller: &accounts.Caller{Store: st, Upstream: up},
	}

	err := svc.fetchContentViaShareChapter(ctx, user.ID, acc.ID, "MP_WXS_1_review")
	if err == nil {
		t.Fatal("expected missing content error")
	}
	if got := atomic.LoadInt32(&shareChapterCalls); got != 1 {
		t.Fatalf("shareChapter fallback should use one request, got %d", got)
	}
	want := []string{"/review/single", "/book/shareChapter"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("paths=%v want=%v", paths, want)
	}
}

func TestFetchLatestWarmsBookOpenBeforeArticles(t *testing.T) {
	withFastBookOpenWarmup(t)

	ctx := context.Background()
	st, user, sub := newArticleTestStore(t)
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

	var paths []string
	var readInfoVID string
	up := articleTestUpstreamClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxy/weread/call" {
			http.NotFound(w, r)
			return
		}
		var req upstream.CallReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode call req: %v", err)
		}
		paths = append(paths, req.Path)
		if req.Path == "/book/readinfo" {
			readInfoVID = req.Query["vid"]
		}
		writeArticleJSON(t, w, upstream.CallResp{
			Status:  http.StatusOK,
			Headers: map[string]string{},
			Body:    json.RawMessage(`{"errcode":0,"reviews":[{"subReviews":[{"review":{"reviewId":"MP_WXS_1_r1","createTime":1778580000,"mpInfo":{"title":"t","content":"s","time":1778580000,"originalId":"x"}}}]}]}`),
		})
	})

	svc := &Service{
		Store:  st,
		Caller: &accounts.Caller{Store: st, Upstream: up},
		Mode:   "summary",
	}
	if _, err := svc.FetchLatest(ctx, user.ID, sub.ID); err != nil {
		t.Fatalf("FetchLatest: %v", err)
	}

	wantPrefix := []string{"/book/info", "/book/getProgress", "/book/readinfo", "/book/detailinfo", "/book/articles"}
	if len(paths) < len(wantPrefix) {
		t.Fatalf("paths=%v want prefix=%v", paths, wantPrefix)
	}
	for i := range wantPrefix {
		if paths[i] != wantPrefix[i] {
			t.Fatalf("paths=%v want prefix=%v", paths, wantPrefix)
		}
	}
	if readInfoVID != "565310662" {
		t.Fatalf("readinfo vid=%q want 565310662", readInfoVID)
	}
}

func TestFetchLatestUsesLightProbeDuringRateLimitRecovery(t *testing.T) {
	ctx := context.Background()
	st, user, sub := newArticleTestStore(t)
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
	if _, err := st.DB().ExecContext(ctx, `
		UPDATE weread_accounts SET last_err = 'errcode=-2041 -2041' WHERE id = ?
	`, acc.ID); err != nil {
		t.Fatalf("set recovery state: %v", err)
	}

	var articleCount string
	var shareChapterCalls int32
	up := articleTestUpstreamClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxy/weread/call" {
			http.NotFound(w, r)
			return
		}
		var req upstream.CallReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode call req: %v", err)
		}
		switch req.Path {
		case "/mobileSync":
			writeArticleJSON(t, w, upstream.CallResp{
				Status:  http.StatusOK,
				Headers: map[string]string{},
				Body:    json.RawMessage(`{"errcode":0}`),
			})
		case "/shelf/sync":
			writeArticleJSON(t, w, upstream.CallResp{
				Status:  http.StatusOK,
				Headers: map[string]string{},
				Body:    json.RawMessage(`{"errcode":0}`),
			})
		case "/book/articles":
			articleCount = req.Query["count"]
			writeArticleJSON(t, w, upstream.CallResp{
				Status:  http.StatusOK,
				Headers: map[string]string{},
				Body:    json.RawMessage(`{"errcode":0,"reviews":[{"subReviews":[{"review":{"reviewId":"MP_WXS_1_r1","createTime":1778580000,"mpInfo":{"title":"t","content":"s","time":1778580000,"originalId":"x"}}}]}]}`),
			})
		case "/book/shareChapter":
			atomic.AddInt32(&shareChapterCalls, 1)
			writeArticleJSON(t, w, upstream.CallResp{
				Status:  http.StatusOK,
				Headers: map[string]string{},
				Body:    json.RawMessage(`{"errcode":0,"data":{"content":"full"}}`),
			})
		default:
			t.Fatalf("unexpected upstream path %s", req.Path)
		}
	})

	svc := &Service{
		Store:  st,
		Caller: &accounts.Caller{Store: st, Upstream: up},
		Mode:   "full",
	}
	n, err := svc.FetchLatest(ctx, user.ID, sub.ID)
	if err != nil {
		t.Fatalf("FetchLatest: %v", err)
	}
	if n != 1 {
		t.Fatalf("new count=%d want 1", n)
	}
	if articleCount != "5" {
		t.Fatalf("recovery probe count=%q want 5", articleCount)
	}
	if got := atomic.LoadInt32(&shareChapterCalls); got != 0 {
		t.Fatalf("recovery probe should not chase full content, got %d shareChapter calls", got)
	}
}

func TestFetchLatestRecordsSourceRateLimitLog(t *testing.T) {
	ctx := context.Background()
	st, user, sub := newArticleTestStore(t)
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

	up := articleTestUpstreamClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxy/weread/call" {
			http.NotFound(w, r)
			return
		}
		writeArticleJSON(t, w, upstream.CallResp{
			Status:  http.StatusOK,
			Headers: map[string]string{},
			Body:    json.RawMessage(`{"errcode":-2041,"errmsg":"-2041"}`),
		})
	})

	svc := &Service{
		Store:  st,
		Caller: &accounts.Caller{Store: st, Upstream: up},
	}
	_, err := svc.FetchLatest(ctx, user.ID, sub.ID)
	if !errors.Is(err, accounts.ErrSearchRateLimited) {
		t.Fatalf("FetchLatest err=%v want ErrSearchRateLimited", err)
	}

	events, err := st.ListFetchEvents(ctx, user.ID, 10, 0, true)
	if err != nil {
		t.Fatalf("ListFetchEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len=%d want 1", len(events))
	}
	if events[0].Chain != "source" || events[0].SubscriptionID != sub.ID || events[0].ErrorCode != "-2041" {
		t.Fatalf("event=(chain:%q sub:%d code:%q), want source/%d/-2041",
			events[0].Chain, events[0].SubscriptionID, events[0].ErrorCode, sub.ID)
	}
}

func newArticleTestStore(t *testing.T) (*store.Store, *model.User, *model.Subscription) {
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
	sub := &model.Subscription{
		UserID: user.ID,
		BookID: "MP_WXS_1",
		Alias:  "sub",
	}
	if err := st.CreateSubscription(ctx, sub); err != nil {
		t.Fatalf("CreateSubscription: %v", err)
	}
	return st, user, sub
}

func articleTestUpstreamClient(t *testing.T, h http.HandlerFunc) *upstream.Client {
	t.Helper()
	c := upstream.New("http://upstream.test", "id", "secret")
	c.HTTP = &http.Client{Transport: articleRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		rr := newArticleResponseRecorder()
		h(rr, req)
		return rr.result(req), nil
	})}
	return c
}

type articleRoundTripFunc func(*http.Request) (*http.Response, error)

func (f articleRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type articleResponseRecorder struct {
	header http.Header
	body   strings.Builder
	code   int
}

func newArticleResponseRecorder() *articleResponseRecorder {
	return &articleResponseRecorder{header: make(http.Header), code: http.StatusOK}
}

func (r *articleResponseRecorder) Header() http.Header {
	return r.header
}

func (r *articleResponseRecorder) Write(b []byte) (int, error) {
	return r.body.Write(b)
}

func (r *articleResponseRecorder) WriteHeader(statusCode int) {
	r.code = statusCode
}

func (r *articleResponseRecorder) result(req *http.Request) *http.Response {
	return &http.Response{
		StatusCode: r.code,
		Status:     http.StatusText(r.code),
		Header:     r.header.Clone(),
		Body:       io.NopCloser(strings.NewReader(r.body.String())),
		Request:    req,
	}
}

func writeArticleJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func withFastBookOpenWarmup(t *testing.T) {
	t.Helper()
	oldMin, oldMax := bookOpenSleepMin, bookOpenSleepMax
	bookOpenSleepMin, bookOpenSleepMax = 0, 0
	t.Cleanup(func() {
		bookOpenSleepMin, bookOpenSleepMax = oldMin, oldMax
	})
}
