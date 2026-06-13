package subs

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cry0404/MyWechatRss/internal/accounts"
	"github.com/cry0404/MyWechatRss/internal/crypto"
	"github.com/cry0404/MyWechatRss/internal/model"
	"github.com/cry0404/MyWechatRss/internal/store"
	"github.com/cry0404/MyWechatRss/internal/upstream"
)

func TestSearchUsesInitialSIDThenMPScopedSearch(t *testing.T) {
	ctx := context.Background()
	st, user := newSubsTestStore(t)

	var calls []upstream.CallReq
	up := upstream.New("http://upstream.test", "test-key", "test-secret")
	up.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/proxy/weread/call" {
			return newSubsHTTPResponse(http.StatusNotFound, `not found`), nil
		}
		var req upstream.CallReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode call req: %v", err)
		}
		calls = append(calls, req)

		switch len(calls) {
		case 1:
			return newSubsJSONResponse(t, upstream.CallResp{
				Status:  http.StatusOK,
				Headers: map[string]string{},
				Body: json.RawMessage(`{
					"errcode": 0,
					"sid": "sid-123",
					"results": [{
						"type": 17,
						"books": [{"bookInfo": {"bookId": "330001", "title": "普通书", "author": "作者"}}]
					}]
				}`),
			}), nil
		case 2:
			return newSubsJSONResponse(t, upstream.CallResp{
				Status:  http.StatusOK,
				Headers: map[string]string{},
				Body: json.RawMessage(`{
					"errcode": 0,
					"books": [{
						"bookInfo": {
							"bookId": "MP_WXS_3271041950",
							"title": "新智元",
							"author": "公众号",
							"cover": "https://example.test/cover.jpg"
						}
					}]
				}`),
			}), nil
		default:
			t.Fatalf("unexpected call #%d: %+v", len(calls), req)
			return nil, nil
		}
	})}

	svc := NewService(st, &accounts.Caller{
		Store:    st,
		Upstream: up,
	})

	items, err := svc.Search(ctx, user.ID, " 新智元 ")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(items) != 1 || items[0].BookID != "MP_WXS_3271041950" {
		t.Fatalf("items=%+v", items)
	}
	if len(calls) != 2 {
		t.Fatalf("got %d calls, want 2", len(calls))
	}

	first := calls[0]
	if first.Path != "/store/search" ||
		first.Query["keyword"] != "新智元" ||
		first.Query["count"] != "10" ||
		first.Query["rnVersion"] != "6" ||
		first.Query["v"] != "3" {
		t.Fatalf("initial search query=%v path=%s", first.Query, first.Path)
	}

	second := calls[1]
	if second.Path != "/store/search" ||
		second.Query["keyword"] != "新智元" ||
		second.Query["count"] != "15" ||
		second.Query["scope"] != "2" ||
		second.Query["sid"] != "sid-123" ||
		second.Query["type"] != "0" ||
		second.Query["v"] != "2" {
		t.Fatalf("scoped search query=%v path=%s", second.Query, second.Path)
	}
}

func newSubsTestStore(t *testing.T) (*store.Store, *model.User) {
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
		UserID: user.ID,
		VID:    565310662,
		SKey:   "skey",
		Status: model.AccountActive,
	}
	if err := st.CreateAccount(ctx, acc); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	return st, user
}

func writeSubsJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func newSubsJSONResponse(t *testing.T, v any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		t.Fatalf("encode response: %v", err)
	}
	return newSubsHTTPResponse(http.StatusOK, buf.String())
}

func newSubsHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
