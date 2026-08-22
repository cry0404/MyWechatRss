package store_test

import (
	"context"
	"testing"

	"github.com/cry0404/MyWechatRss/internal/model"
)

func TestCountArticlesByUserSince(t *testing.T) {
	st, user, sub := newStoreTestSubscription(t)
	ctx := context.Background()
	for _, article := range []*model.Article{
		{BookID: sub.BookID, ReviewID: "old", Title: "old", PublishAt: 99},
		{BookID: sub.BookID, ReviewID: "new-1", Title: "new 1", PublishAt: 100},
		{BookID: sub.BookID, ReviewID: "new-2", Title: "new 2", PublishAt: 101},
	} {
		if _, err := st.UpsertArticle(ctx, article); err != nil {
			t.Fatalf("UpsertArticle %s: %v", article.ReviewID, err)
		}
	}

	count, err := st.CountArticlesByUserSince(ctx, user.ID, 100)
	if err != nil {
		t.Fatalf("CountArticlesByUserSince: %v", err)
	}
	if count != 2 {
		t.Fatalf("count=%d want 2", count)
	}

	if _, err := st.DB().ExecContext(ctx, `UPDATE subscriptions SET disabled = 1 WHERE id = ?`, sub.ID); err != nil {
		t.Fatalf("disable subscription: %v", err)
	}
	count, err = st.CountArticlesByUserSince(ctx, user.ID, 0)
	if err != nil {
		t.Fatalf("CountArticlesByUserSince disabled: %v", err)
	}
	if count != 0 {
		t.Fatalf("disabled count=%d want 0", count)
	}
}
