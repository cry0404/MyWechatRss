package keepalive

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/cry0404/MyWechatRss/internal/accounts"
	"github.com/cry0404/MyWechatRss/internal/crypto"
	"github.com/cry0404/MyWechatRss/internal/model"
	"github.com/cry0404/MyWechatRss/internal/store"
	"github.com/cry0404/MyWechatRss/internal/upstream"
)

func TestRunOnceUsesShelfSyncHeartbeat(t *testing.T) {
	ctx := context.Background()
	codec, err := crypto.New("test-secret-with-enough-length")
	if err != nil {
		t.Fatalf("crypto.New: %v", err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"), codec)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	user := &model.User{Username: "u", Email: "u@example.com", PasswordHash: "hash"}
	if err := st.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	acc := &model.WeReadAccount{
		UserID:       user.ID,
		VID:          565310662,
		SKey:         "skey",
		RefreshToken: "refresh-token",
		Cookies:      map[string]string{"wr_vid": "565310662"},
		Status:       model.AccountActive,
		DeviceID:     "dev",
		InstallID:    "install",
		DeviceName:   "device",
	}
	if err := st.CreateAccount(ctx, acc); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if err := st.MarkAccountOK(ctx, acc.ID); err != nil {
		t.Fatalf("MarkAccountOK: %v", err)
	}

	var gotPath string
	var gotQuery map[string]string
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/proxy/weread/call" {
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(bytes.NewReader(nil)), Header: http.Header{}}, nil
		}
		var req upstream.CallReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return nil, err
		}
		gotPath = req.Path
		gotQuery = req.Query
		var body bytes.Buffer
		if err := json.NewEncoder(&body).Encode(upstream.CallResp{
			Status:  http.StatusOK,
			Headers: map[string]string{},
			Body:    json.RawMessage(`{"errcode":0}`),
		}); err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(&body),
		}, nil
	})}

	up := upstream.New("http://wechatread.test", "id", "secret")
	up.HTTP = httpClient
	caller := &accounts.Caller{Store: st, Upstream: up}
	scheduler := NewScheduler(st, caller)
	scheduler.InterAccountSleepMin = 0
	scheduler.InterAccountSleepMax = 0

	scheduler.runOnce(ctx)

	if gotPath != "/shelf/sync" {
		t.Fatalf("heartbeat path=%q want /shelf/sync", gotPath)
	}
	if gotQuery["onlyBookid"] != "1" || gotQuery["synckey"] != "0" {
		t.Fatalf("heartbeat query=%v want onlyBookid=1 synckey=0", gotQuery)
	}
	if time.Since(time.Unix(getOnlyAccountLastOK(t, st, user.ID), 0)) > time.Minute {
		t.Fatal("heartbeat should keep account ok")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func getOnlyAccountLastOK(t *testing.T, st *store.Store, userID int64) int64 {
	t.Helper()
	accs, err := st.ListAccountsByUser(context.Background(), userID)
	if err != nil {
		t.Fatalf("ListAccountsByUser: %v", err)
	}
	if len(accs) != 1 {
		t.Fatalf("accounts=%d want 1", len(accs))
	}
	return accs[0].LastOkAt
}
