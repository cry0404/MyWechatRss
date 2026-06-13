package rss

import (
	"strings"
	"testing"
	"time"

	"github.com/cry0404/MyWechatRss/internal/model"
)

func TestRenderSubscriptionSummaryOnlyOmitsContentEncoded(t *testing.T) {
	sub := &model.Subscription{BookID: "MP_WXS_1", Alias: "Feed"}
	article := &model.Article{
		BookID:      sub.BookID,
		ReviewID:    "review-1",
		Title:       "Title",
		Summary:     "Short summary",
		ContentHTML: "<p>Full article body</p>",
		URL:         "https://mp.weixin.qq.com/s/example",
		PublishAt:   time.Now().Unix(),
	}

	out, err := RenderSubscription(sub, []*model.Article{article}, RenderOptions{
		PublicBaseURL: "https://example.com",
		SelfURL:       "https://example.com/rss/feed.xml",
		SummaryOnly:   true,
	})
	if err != nil {
		t.Fatalf("RenderSubscription: %v", err)
	}
	xml := string(out)
	if strings.Contains(xml, "content:encoded") || strings.Contains(xml, "Full article body") {
		t.Fatalf("summary-only feed must not include full content: %s", xml)
	}
	if !strings.Contains(xml, "Short summary") {
		t.Fatalf("summary-only feed should still include description: %s", xml)
	}
}
