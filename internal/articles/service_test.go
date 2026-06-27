package articles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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

func TestFetchLatestUsesWebMpArticlesWithoutAppArticleWarmup(t *testing.T) {
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

	var appCalls int32
	up := articleTestUpstreamClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxy/weread/call" {
			http.NotFound(w, r)
			return
		}
		var req upstream.CallReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode call req: %v", err)
		}
		atomic.AddInt32(&appCalls, 1)
		t.Fatalf("unexpected app upstream call %s", req.Path)
		writeArticleJSON(t, w, upstream.CallResp{
			Status:  http.StatusOK,
			Headers: map[string]string{},
			Body:    json.RawMessage(`{"errcode":0}`),
		})
	})
	withWereadWebHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/web/mp/articles" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("bookId"); got != sub.BookID {
			t.Fatalf("bookId=%q want %q", got, sub.BookID)
		}
		if got := r.Header.Get("Cookie"); !strings.Contains(got, "wr_vid=565310662") || !strings.Contains(got, "wr_skey=skey") {
			t.Fatalf("cookie header missing wr_vid/wr_skey: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if got := r.URL.Query().Get("offset"); got == "1" {
			_, _ = io.WriteString(w, `{"synckey":1782304237,"reviews":[]}`)
			return
		} else if got != "0" {
			t.Fatalf("offset=%q want 0 or 1", got)
		}
		_, _ = io.WriteString(w, `{
			"synckey": 1782304237,
			"reviews": [{
				"createTime": 1778580000,
				"subReviews": [{
					"reviewId": "MP_WXS_1_r1",
					"review": {
						"reviewId": "MP_WXS_1_r1",
						"createTime": 1778580000,
						"mpInfo": {
							"title": "web title",
							"content": "web summary",
							"time": 1778580000,
							"originalId": "abc~def",
							"pic_url": "https://example.test/pic.jpg",
							"readNum": 12,
							"likeNum": 3
						}
					}
				}]
			}]
		}`)
	})

	svc := &Service{
		Store:  st,
		Caller: &accounts.Caller{Store: st, Upstream: up},
		Mode:   "summary",
	}
	n, err := svc.FetchLatest(ctx, user.ID, sub.ID)
	if err != nil {
		t.Fatalf("FetchLatest: %v", err)
	}
	if n != 1 {
		t.Fatalf("new count=%d want 1", n)
	}
	if got := atomic.LoadInt32(&appCalls); got != 0 {
		t.Fatalf("unexpected app upstream calls=%d", got)
	}

	gotSub, err := st.GetSubscription(ctx, user.ID, sub.ID)
	if err != nil {
		t.Fatalf("GetSubscription: %v", err)
	}
	if gotSub.ArticleSynckey != 1782304237 {
		t.Fatalf("article synckey=%d want 1782304237", gotSub.ArticleSynckey)
	}

	article, err := st.GetArticleByReviewID(ctx, "MP_WXS_1_r1")
	if err != nil {
		t.Fatalf("GetArticleByReviewID: %v", err)
	}
	if article.Title != "web title" || article.Summary != "web summary" || article.URL != "https://mp.weixin.qq.com/s/abc_def" {
		t.Fatalf("article parsed from web response = %+v", article)
	}
	events, err := st.ListFetchEvents(ctx, user.ID, 10, 0, false)
	if err != nil {
		t.Fatalf("ListFetchEvents: %v", err)
	}
	if len(events) != 1 || events[0].Chain != "web/mp/articles" || !events[0].Success {
		t.Fatalf("events=%+v want one successful web/mp/articles source event", events)
	}
}

func TestFetchLatestDoesNotFetchSecondPageWhenFirstPageIsSparse(t *testing.T) {
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

	var articleCalls int32
	withWereadWebHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/web/mp/articles" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("offset"); got != "0" {
			t.Fatalf("unexpected second page request offset=%q", got)
		}
		atomic.AddInt32(&articleCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"synckey": 1782304237,
			"reviews": [{
				"createTime": 1778580000,
				"subReviews": [{
					"reviewId": "MP_WXS_1_r1",
					"review": {
						"reviewId": "MP_WXS_1_r1",
						"createTime": 1778580000,
						"mpInfo": {
							"title": "web title",
							"content": "web summary",
							"time": 1778580000,
							"originalId": "abc~def"
						}
					}
				}]
			}]
		}`)
	})

	svc := &Service{Store: st, Mode: "summary"}
	if _, err := svc.FetchLatest(ctx, user.ID, sub.ID); err != nil {
		t.Fatalf("FetchLatest: %v", err)
	}
	if got := atomic.LoadInt32(&articleCalls); got != 1 {
		t.Fatalf("article list calls=%d want 1", got)
	}
}

func TestFetchLatestSpacesSecondArticleListPage(t *testing.T) {
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

	oldMin, oldMax := firstFetchSleepMin, firstFetchSleepMax
	firstFetchSleepMin, firstFetchSleepMax = 20*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() {
		firstFetchSleepMin, firstFetchSleepMax = oldMin, oldMax
	})

	var firstAt time.Time
	var gap time.Duration
	withWereadWebHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/web/mp/articles" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("offset") {
		case "0":
			firstAt = time.Now()
			_, _ = io.WriteString(w, fullArticleListResponse(20))
		case "20":
			gap = time.Since(firstAt)
			_, _ = io.WriteString(w, `{"synckey":1782304238,"reviews":[]}`)
		default:
			t.Fatalf("unexpected offset=%q", r.URL.Query().Get("offset"))
		}
	})

	svc := &Service{Store: st, Mode: "summary"}
	if _, err := svc.FetchLatest(ctx, user.ID, sub.ID); err != nil {
		t.Fatalf("FetchLatest: %v", err)
	}
	if gap < 15*time.Millisecond {
		t.Fatalf("second page gap=%s want at least 15ms", gap)
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
	withWereadWebHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/web/mp/articles" {
			http.NotFound(w, r)
			return
		}
		articleCount = r.URL.Query().Get("count")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"reviews":[{"subReviews":[{"review":{"reviewId":"MP_WXS_1_r1","createTime":1778580000,"mpInfo":{"title":"t","content":"s","time":1778580000,"originalId":"x"}}}]}]}`)
	})
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

	withWereadWebHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/web/mp/articles" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"errcode":-2041,"errmsg":"-2041"}`)
	})

	svc := &Service{
		Store: st,
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
	if events[0].Chain != "web/mp/articles" || events[0].SubscriptionID != sub.ID || events[0].ErrorCode != "-2041" {
		t.Fatalf("event=(chain:%q sub:%d code:%q), want web/mp/articles/%d/-2041",
			events[0].Chain, events[0].SubscriptionID, events[0].ErrorCode, sub.ID)
	}
}

func TestFetchLatestRefreshesAndDefersAfterWebSessionExpired(t *testing.T) {
	ctx := context.Background()
	st, user, sub := newArticleTestStore(t)
	acc := &model.WeReadAccount{
		UserID:       user.ID,
		VID:          565310662,
		SKey:         "expired-skey",
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

	var refreshCalls int32
	up := articleTestUpstreamClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxy/weread/login/refresh" {
			http.NotFound(w, r)
			return
		}
		atomic.AddInt32(&refreshCalls, 1)
		writeArticleJSON(t, w, upstream.LoginRefreshResp{
			Credential: &upstream.Credential{
				VID:          565310662,
				SKey:         "fresh-skey",
				RefreshToken: "refresh-token",
			},
		})
	})
	withWereadWebHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/web/mp/articles" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"errcode":-2012,"errmsg":"登录超时"}`)
	})

	svc := &Service{
		Store:  st,
		Caller: &accounts.Caller{Store: st, Upstream: up},
	}
	_, err := svc.FetchLatest(ctx, user.ID, sub.ID)
	if !errors.Is(err, accounts.ErrHighRiskDeferred) {
		t.Fatalf("FetchLatest err=%v want ErrHighRiskDeferred", err)
	}
	if got := atomic.LoadInt32(&refreshCalls); got != 1 {
		t.Fatalf("refresh calls=%d want 1", got)
	}
	accs, err := st.ListAccountsByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListAccountsByUser: %v", err)
	}
	if len(accs) != 1 {
		t.Fatalf("accounts=%d want 1", len(accs))
	}
	if accs[0].SKey != "fresh-skey" || accs[0].Status != model.AccountActive || accs[0].LastErr != "" {
		t.Fatalf("account after refresh=%+v", accs[0])
	}
	events, err := st.ListFetchEvents(ctx, user.ID, 10, 0, false)
	if err != nil {
		t.Fatalf("ListFetchEvents: %v", err)
	}
	if len(events) != 1 || events[0].Chain != "web/mp/articles" || events[0].ErrorCode != "-2012" {
		t.Fatalf("events=%+v want one web/mp/articles -2012 event", events)
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

func withWereadWebHandler(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	oldBaseURL := wereadWebBaseURL
	oldClient := webContentClient
	wereadWebBaseURL = "https://weread.test"
	webContentClient = &http.Client{Transport: articleRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		rr := newArticleResponseRecorder()
		h(rr, req)
		return rr.result(req), nil
	})}
	t.Cleanup(func() {
		wereadWebBaseURL = oldBaseURL
		webContentClient = oldClient
	})
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

func fullArticleListResponse(n int) string {
	var b strings.Builder
	b.WriteString(`{"synckey":1782304237,"reviews":[{"createTime":1778580000,"subReviews":[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"reviewId":"MP_WXS_1_r%d","review":{"reviewId":"MP_WXS_1_r%d","createTime":%d,"mpInfo":{"title":"t%d","content":"s%d","time":%d,"originalId":"x%d"}}}`,
			i, i, 1778580000+i, i, i, 1778580000+i, i)
	}
	b.WriteString(`]}]}`)
	return b.String()
}

func withFastBookOpenWarmup(t *testing.T) {
	t.Helper()
	oldMin, oldMax := bookOpenSleepMin, bookOpenSleepMax
	bookOpenSleepMin, bookOpenSleepMax = 0, 0
	t.Cleanup(func() {
		bookOpenSleepMin, bookOpenSleepMax = oldMin, oldMax
	})
}
