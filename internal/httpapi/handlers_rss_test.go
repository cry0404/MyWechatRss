package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/cry0404/MyWechatRss/internal/crypto"
	"github.com/cry0404/MyWechatRss/internal/model"
	"github.com/cry0404/MyWechatRss/internal/rss"
	"github.com/cry0404/MyWechatRss/internal/store"
)

func TestRSSHandlersLimitDefaultFeedItems(t *testing.T) {
	gin.SetMode(gin.TestMode)
	st, user, sub := newRSSHandlerTestStore(t)
	ctx := context.Background()

	base := int64(1_800_000_000)
	for i := 0; i < 40; i++ {
		_, err := st.UpsertArticle(ctx, &model.Article{
			BookID:      sub.BookID,
			ReviewID:    sub.BookID + "_" + string(rune('a'+i)),
			Title:       "article",
			Summary:     "summary",
			ContentHTML: strings.Repeat("content", 50),
			PublishAt:   base + int64(i),
			FetchedAt:   base + int64(i),
		})
		if err != nil {
			t.Fatalf("UpsertArticle %d: %v", i, err)
		}
	}

	enc := rss.NewFeedIDEncoder("test-salt")
	h := &RSSHandlers{
		Store:         st,
		FeedEncoder:   enc,
		PublicBaseURL: "https://example.com",
	}
	r := gin.New()
	r.GET("/rss/:feedId", h.Serve)

	tests := []struct {
		name string
		url  string
		want int
	}{
		{
			name: "aggregate",
			url:  "/rss/" + enc.Encode(user.ID, 0) + ".xml",
			want: 30,
		},
		{
			name: "single",
			url:  "/rss/" + enc.Encode(user.ID, sub.ID) + ".xml",
			want: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if got := strings.Count(rec.Body.String(), "<item>"); got != tt.want {
				t.Fatalf("item count=%d want %d", got, tt.want)
			}
		})
	}
}

func newRSSHandlerTestStore(t *testing.T) (*store.Store, *model.User, *model.Subscription) {
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
		Alias:  "Feed",
	}
	if err := st.CreateSubscription(ctx, sub); err != nil {
		t.Fatalf("CreateSubscription: %v", err)
	}
	return st, user, sub
}
