package accounts

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cry0404/MyWechatRss/internal/crypto"
	"github.com/cry0404/MyWechatRss/internal/model"
	"github.com/cry0404/MyWechatRss/internal/store"
	"github.com/cry0404/MyWechatRss/internal/upstream"
)

func TestDoCooldownsBookArticles401WhenRefreshDebounced(t *testing.T) {
	ctx := context.Background()
	st, user, acc := newCallerTestStore(t)

	var refreshCalls int32
	up := testUpstreamClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/proxy/weread/call":
			writeJSON(t, w, upstream.CallResp{
				Status:  http.StatusUnauthorized,
				Headers: map[string]string{},
				Body:    json.RawMessage(`{"errcode":-2012,"errmsg":"expired"}`),
			})
		case "/proxy/weread/login/refresh":
			atomic.AddInt32(&refreshCalls, 1)
			writeJSON(t, w, upstream.LoginRefreshResp{
				Status: "ok",
				Credential: &upstream.Credential{
					VID:          acc.VID,
					SKey:         "new-skey",
					RefreshToken: acc.RefreshToken,
				},
			})
		default:
			http.NotFound(w, r)
		}
	})

	cr := &Caller{Store: st, Upstream: up}
	if !cr.refreshGuard.allow(acc.ID, time.Now()) {
		t.Fatal("expected initial refresh guard allowance")
	}

	_, err := cr.Do(ctx, user.ID, CallOptions{
		Method: http.MethodGet,
		Path:   "/book/articles",
		Query:  map[string]string{"bookId": "MP_WXS_1", "count": "20"},
	})
	if err == nil {
		t.Fatal("expected transient auth/cooldown error")
	}
	if atomic.LoadInt32(&refreshCalls) != 0 {
		t.Fatalf("debounced refresh should not call refresh endpoint, got %d", refreshCalls)
	}

	got := getOnlyAccount(t, st, user.ID)
	if got.Status == model.AccountDead {
		t.Fatalf("business-path 401 after a recent refresh must not mark account dead: last_err=%q", got.LastErr)
	}
	if got.Status != model.AccountCooldown {
		t.Fatalf("expected account cooldown for business-path 401, got %q last_err=%q", got.Status, got.LastErr)
	}
}

func TestDoCooldownsRateLimitedBookArticlesWithoutRefreshRetry(t *testing.T) {
	ctx := context.Background()
	st, user, _ := newCallerTestStore(t)

	var callCalls int32
	var refreshCalls int32
	up := testUpstreamClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/proxy/weread/call":
			atomic.AddInt32(&callCalls, 1)
			writeJSON(t, w, upstream.CallResp{
				Status:  http.StatusOK,
				Headers: map[string]string{},
				Body:    json.RawMessage(`{"errcode":-2041,"errmsg":"-2041"}`),
			})
		case "/proxy/weread/login/refresh":
			atomic.AddInt32(&refreshCalls, 1)
			writeJSON(t, w, upstream.LoginRefreshResp{
				Status:     "ok",
				Credential: &upstream.Credential{VID: 565310662, SKey: "new-skey"},
			})
		default:
			http.NotFound(w, r)
		}
	})

	cr := &Caller{Store: st, Upstream: up}

	_, err := cr.Do(ctx, user.ID, CallOptions{
		Method: http.MethodGet,
		Path:   "/book/articles",
		Query:  map[string]string{"bookId": "MP_WXS_1", "count": "20"},
	})
	if err == nil || !strings.Contains(err.Error(), "-2041") {
		t.Fatalf("expected -2041 error, got %v", err)
	}
	if got := atomic.LoadInt32(&callCalls); got != 1 {
		t.Fatalf("rate-limited request should not be retried immediately, got %d calls", got)
	}
	if got := atomic.LoadInt32(&refreshCalls); got != 0 {
		t.Fatalf("-2041 should not trigger refresh, got %d refresh calls", got)
	}

	got := getOnlyAccount(t, st, user.ID)
	if got.Status != model.AccountCooldown {
		t.Fatalf("expected cooldown after -2041, got %q last_err=%q", got.Status, got.LastErr)
	}
}

func TestDoMarksDeadForAuthProbeWhenRefreshFails(t *testing.T) {
	ctx := context.Background()
	st, user, _ := newCallerTestStore(t)

	up := testUpstreamClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/proxy/weread/call":
			writeJSON(t, w, upstream.CallResp{
				Status:  http.StatusUnauthorized,
				Headers: map[string]string{},
				Body:    json.RawMessage(`{"errcode":-2012,"errmsg":"expired"}`),
			})
		case "/proxy/weread/login/refresh":
			http.Error(w, `{"errcode":-2012,"errmsg":"refresh expired"}`, http.StatusBadGateway)
		default:
			http.NotFound(w, r)
		}
	})

	cr := &Caller{Store: st, Upstream: up}

	_, err := cr.Do(ctx, user.ID, CallOptions{
		Method: http.MethodGet,
		Path:   "/device/sessionlist",
		Query:  map[string]string{"deviceId": "dev", "onlyCnt": "1"},
	})
	if err == nil {
		t.Fatal("expected auth probe failure")
	}

	got := getOnlyAccount(t, st, user.ID)
	if got.Status != model.AccountDead {
		t.Fatalf("expected dead after auth probe refresh failure, got %q last_err=%q", got.Status, got.LastErr)
	}
}

func newCallerTestStore(t *testing.T) (*store.Store, *model.User, *model.WeReadAccount) {
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
		SKey:         "old-skey",
		RefreshToken: "refresh-token",
		Cookies:      map[string]string{"wr_vid": "565310662"},
		Nickname:     "reader",
		Status:       model.AccountActive,
		DeviceID:     "dev",
		InstallID:    "install",
		DeviceName:   "device",
	}
	if err := st.CreateAccount(ctx, acc); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	return st, user, acc
}

func getOnlyAccount(t *testing.T, st *store.Store, userID int64) *model.WeReadAccount {
	t.Helper()
	accs, err := st.ListAccountsByUser(context.Background(), userID)
	if err != nil {
		t.Fatalf("ListAccountsByUser: %v", err)
	}
	if len(accs) != 1 {
		t.Fatalf("expected one account, got %d", len(accs))
	}
	return accs[0]
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func testUpstreamClient(t *testing.T, h http.HandlerFunc) *upstream.Client {
	t.Helper()
	c := upstream.New("http://upstream.test", "id", "secret")
	c.HTTP = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		rr := newResponseRecorder()
		h(rr, req)
		return rr.result(req), nil
	})}
	return c
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type responseRecorder struct {
	header http.Header
	body   strings.Builder
	code   int
}

func newResponseRecorder() *responseRecorder {
	return &responseRecorder{header: make(http.Header), code: http.StatusOK}
}

func (r *responseRecorder) Header() http.Header {
	return r.header
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	return r.body.Write(b)
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.code = statusCode
}

func (r *responseRecorder) result(req *http.Request) *http.Response {
	return &http.Response{
		StatusCode: r.code,
		Status:     http.StatusText(r.code),
		Header:     r.header.Clone(),
		Body:       io.NopCloser(strings.NewReader(r.body.String())),
		Request:    req,
	}
}
