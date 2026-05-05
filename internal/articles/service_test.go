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

	var callCalls int32
	up := articleTestUpstreamClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxy/weread/call" {
			http.NotFound(w, r)
			return
		}
		atomic.AddInt32(&callCalls, 1)
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
	if got := atomic.LoadInt32(&callCalls); got != 1 {
		t.Fatalf("shareChapter fallback should use one request, got %d", got)
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
