import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { formatDate, formatRelativeTime, copyToClipboard } from "@/lib/utils";
import { api, type Article, type Subscription } from "@/lib/api";
import { formatInterval, formatFetchWindowLine } from "@/lib/schedule";
import { SafeImg } from "@/components/SafeImg";
import { SubscriptionScheduleModal } from "@/components/SubscriptionScheduleModal";
import { useAlertStore } from "@/stores/alertStore";
import { toUserMessage } from "@/lib/userMessage";

const PAGE_SIZE = 20;

export default function SubscriptionDetailPage() {
  const showAlert = useAlertStore((s) => s.show);
  const { id } = useParams<{ id: string }>();
  const subscriptionId = Number(id);
  const [offset, setOffset] = useState(0);
  const [copied, setCopied] = useState(false);
  const [scheduleOpen, setScheduleOpen] = useState(false);
  const queryClient = useQueryClient();

  const { data: subscription, isLoading: subLoading } = useQuery({
    queryKey: ["subscription", subscriptionId],
    queryFn: async () => {
      const subs = await api.getSubscriptions();
      return subs.find((s) => s.id === subscriptionId) ?? null;
    },
  });

  const { data: articles, isLoading: articlesLoading } = useQuery({
    queryKey: ["articles", subscriptionId, offset],
    queryFn: () => api.getArticles(subscriptionId, offset),
  });

  const refreshMutation = useMutation({
    mutationFn: () => api.refreshSubscription(subscriptionId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["articles", subscriptionId] });
      queryClient.invalidateQueries({ queryKey: ["subscriptions"] });
      queryClient.invalidateQueries({ queryKey: ["subscription", subscriptionId] });
    },
    onError: (err) => showAlert(toUserMessage(err)),
  });

  const handleCopyFeed = async (sub: Subscription) => {
    const url = `${window.location.origin}/rss/${sub.feed_id}`;
    const ok = await copyToClipboard(url);
    if (ok) {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  const handleLoadMore = () => {
    setOffset((prev) => prev + PAGE_SIZE);
  };

  if (subLoading) {
    return (
      <div className="flex items-center justify-center py-20">
        <Loader2 className="w-6 h-6 animate-spin" style={{ color: "var(--color-ink-muted)" }} />
      </div>
    );
  }

  if (!subscription) {
    return (
      <div className="max-w-2xl mx-auto py-12 text-center">
        <p className="text-sm mb-4" style={{ color: "var(--color-ink-muted)" }}>
          订阅不存在或已删除
        </p>
        <Link to="/subscriptions" className="text-sm underline">
          返回列表
        </Link>
      </div>
    );
  }

  const allArticles = articles ?? [];
  const hasMore = allArticles.length === PAGE_SIZE;

  return (
    <div className="page-enter max-w-2xl mx-auto">
      <Link to="/subscriptions" className="text-xs mb-6 inline-block" style={{ color: "var(--color-ink-muted)" }}>
        返回订阅
      </Link>

      <header className="mb-8 pb-6 border-b-2" style={{ borderColor: "var(--color-border-soft)" }}>
        <div className="flex gap-4 items-start">
          <SafeImg src={subscription.cover_url} alt="" className="w-16 h-16 object-cover shrink-0 rounded-md border-2" style={{ borderColor: "var(--color-border)" }} />
          <div className="flex-1 min-w-0">
            <h1 className="text-3xl font-heading mb-0.5 truncate">{subscription.alias}</h1>
            <p className="text-lg mb-4 truncate" style={{ color: "var(--color-ink-muted)" }}>
              {subscription.mp_name}
            </p>
            <p className="text-sm mb-3" style={{ color: "var(--color-ink-muted)" }}>
              自动间隔 {formatInterval(subscription.fetch_interval_sec)}
              {formatFetchWindowLine(
                subscription.fetch_window_start_min ?? -1,
                subscription.fetch_window_end_min ?? -1
              ) && (
                <>
                  {" "}
                  · 仅{" "}
                  {formatFetchWindowLine(
                    subscription.fetch_window_start_min ?? -1,
                    subscription.fetch_window_end_min ?? -1
                  )}
                </>
              )}
            </p>
            <div className="flex flex-wrap gap-2">
              <button
                type="button"
                onClick={() => handleCopyFeed(subscription)}
                className="text-base px-3 py-1.5 border-2 rounded-full"
                style={{ borderColor: "var(--color-border)" }}
              >
                {copied ? "已复制" : "复制 RSS 链接"}
              </button>
              <button
                type="button"
                onClick={() => setScheduleOpen(true)}
                className="text-base px-3 py-1.5 border-2 rounded-full"
                style={{ borderColor: "var(--color-border)" }}
              >
                抓取计划
              </button>
              <button
                type="button"
                onClick={() => refreshMutation.mutate()}
                disabled={refreshMutation.isPending}
                className="text-base px-3 py-1.5 border-2 rounded-full disabled:opacity-50"
                style={{ borderColor: "var(--color-border)" }}
              >
                {refreshMutation.isPending ? "抓取中…" : "立即抓取"}
              </button>
            </div>
            {refreshMutation.isSuccess && (
              <p className="text-xs mt-2" style={{ color: "var(--color-success)" }}>
                新增 {refreshMutation.data.new_count} 篇
              </p>
            )}
          </div>
        </div>
      </header>

      <h2 className="text-2xl font-heading mb-4">文章</h2>

      {articlesLoading && offset === 0 ? (
        <div className="flex justify-center py-16">
          <Loader2 className="w-6 h-6 animate-spin" style={{ color: "var(--color-ink-muted)" }} />
        </div>
      ) : allArticles.length === 0 ? (
        <p className="text-sm py-10" style={{ color: "var(--color-ink-muted)" }}>
          暂无文章，可先「立即抓取」
        </p>
      ) : (
        <div className="border-t-2" style={{ borderColor: "var(--color-border-soft)" }}>
          {allArticles.map((article) => (
            <ArticleRow key={article.id} article={article} />
          ))}
        </div>
      )}

      {hasMore && (
        <div className="flex justify-center mt-8">
          <button
            type="button"
            onClick={handleLoadMore}
            disabled={articlesLoading}
            className="text-lg px-4 py-2 border-2 rounded-full disabled:opacity-50"
            style={{ borderColor: "var(--color-border)" }}
          >
            {articlesLoading ? "加载中…" : "更多"}
          </button>
        </div>
      )}

      {scheduleOpen && (
        <SubscriptionScheduleModal
          subscription={{
            ...subscription,
            fetch_window_start_min: subscription.fetch_window_start_min ?? -1,
            fetch_window_end_min: subscription.fetch_window_end_min ?? -1,
          }}
          onClose={() => setScheduleOpen(false)}
        />
      )}
    </div>
  );
}

function ArticleRow({ article }: { article: Article }) {
  return (
    <article className="py-5 border-b-2 last:border-b-0" style={{ borderColor: "var(--color-border-soft)" }}>
      <h3 className="text-xl font-heading mb-2 leading-snug">{article.title}</h3>
      {article.summary && (
        <p className="text-lg leading-relaxed mb-3 line-clamp-2" style={{ color: "var(--color-ink-light)" }}>
          {article.summary}
        </p>
      )}
      <p className="text-xs mb-2" style={{ color: "var(--color-ink-muted)" }}>
        {formatDate(article.publish_at)} · {formatRelativeTime(article.publish_at)} · 阅读 {article.read_num.toLocaleString()} · 赞{" "}
        {article.like_num.toLocaleString()}
      </p>
      {article.url && (
        <a
          href={article.url}
          target="_blank"
          rel="noopener noreferrer"
          className="text-xs underline underline-offset-2"
          style={{ color: "var(--color-ink-muted)" }}
        >
          原文
        </a>
      )}
    </article>
  );
}
