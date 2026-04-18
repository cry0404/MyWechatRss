import { useState, useMemo } from "react";
import { Link } from "react-router-dom";
import { SafeImg } from "@/components/SafeImg";
import { useQuery } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { api } from "@/lib/api";
import { formatRelativeTime } from "@/lib/utils";

const PAGE_SIZE = 20;

export default function RSSFeedsPage() {
  const [globalCopied, setGlobalCopied] = useState(false);
  const [displayCount, setDisplayCount] = useState(PAGE_SIZE);

  const { data: subscriptions, isLoading: subsLoading } = useQuery({
    queryKey: ["subscriptions"],
    queryFn: () => api.getSubscriptions(),
  });

  const { data: me } = useQuery({
    queryKey: ["me"],
    queryFn: () => api.getMe(),
  });

  const { data: allArticles, isLoading: articlesLoading } = useQuery({
    queryKey: ["feed-articles", subscriptions?.map((s) => s.id)],
    enabled: !!subscriptions && subscriptions.length > 0,
    queryFn: async () => {
      const lists = await Promise.all(
        subscriptions!.map(async (sub) => {
          try {
            const arts = await api.getArticles(sub.id, 0);
            return arts.map((a) => ({
              ...a,
              _subName: sub.alias,
            }));
          } catch {
            return [];
          }
        })
      );
      return lists.flat().sort((a, b) => b.publish_at - a.publish_at);
    },
    staleTime: 60_000,
  });

  const globalFeedUrl = me?.global_feed_url ?? "";

  const handleCopyGlobal = async () => {
    if (!globalFeedUrl) return;
    try {
      await navigator.clipboard.writeText(globalFeedUrl);
      setGlobalCopied(true);
      setTimeout(() => setGlobalCopied(false), 2000);
    } catch {
      /* ignore */
    }
  };

  const displayed = useMemo(() => (allArticles ?? []).slice(0, displayCount), [allArticles, displayCount]);

  const hasMore = (allArticles?.length ?? 0) > displayCount;
  const isLoading = subsLoading || articlesLoading;

  return (
    <div className="page-enter max-w-2xl mx-auto">
      <header className="mb-8">
        <h1 className="text-4xl font-heading mb-1">文章流</h1>
        <p className="text-lg" style={{ color: "var(--color-ink-muted)" }}>
          全部订阅按发布时间合并
        </p>
      </header>

      {globalFeedUrl && (
        <div className="mb-8 pb-6 border-b-2" style={{ borderColor: "var(--color-border-soft)" }}>
          <p className="text-xs mb-2" style={{ color: "var(--color-ink-muted)" }}>
            聚合 RSS（全部订阅）
          </p>
          <div className="flex flex-col sm:flex-row sm:items-center gap-2">
            <code
              className="text-xs px-2 py-1.5 font-mono truncate flex-1 min-w-0"
              style={{ backgroundColor: "var(--color-bg-muted)" }}
            >
              {globalFeedUrl}
            </code>
            <div className="flex gap-2 shrink-0">
              <button type="button" onClick={handleCopyGlobal} className="btn-primary text-base px-3 py-1.5 rounded-full">
                {globalCopied ? "已复制" : "复制"}
              </button>
              <a href={globalFeedUrl} target="_blank" rel="noopener noreferrer" className="btn-secondary text-base px-3 py-1.5 rounded-full">
                打开
              </a>
            </div>
          </div>
        </div>
      )}

      {isLoading ? (
        <div className="flex justify-center py-20">
          <Loader2 className="w-5 h-5 animate-spin" style={{ color: "var(--color-ink-muted)" }} />
        </div>
      ) : !subscriptions || subscriptions.length === 0 ? (
        <div className="py-16 text-center">
          <p className="text-sm mb-2">暂无订阅</p>
          <Link to="/subscriptions" className="text-xs underline">
            去添加
          </Link>
        </div>
      ) : allArticles && allArticles.length === 0 ? (
        <p className="text-sm py-12" style={{ color: "var(--color-ink-muted)" }}>
          尚无文章，等待抓取或到订阅页手动刷新
        </p>
      ) : (
        <>
          <div className="border-t-2 mb-6" style={{ borderColor: "var(--color-border-soft)" }}>
            {displayed.map((article) => (
              <FeedArticleRow key={`${article.id}-${article.review_id}`} article={article} />
            ))}
          </div>

          {hasMore && (
            <div className="flex justify-center">
              <button
                type="button"
                onClick={() => setDisplayCount((c) => c + PAGE_SIZE)}
                className="text-lg px-4 py-2 border-2 rounded-full"
                style={{ borderColor: "var(--color-border)" }}
              >
                更多
              </button>
            </div>
          )}
        </>
      )}
    </div>
  );
}

function FeedArticleRow({
  article,
}: {
  article: {
    id: number;
    title: string;
    summary: string;
    url?: string;
    cover_url?: string;
    publish_at: number;
    read_num: number;
    like_num: number;
    _subName: string;
  };
}) {
  const thumb = article.cover_url?.trim();

  const inner = (
    <div className="flex gap-3 items-start min-w-0">
      {thumb ? (
        <SafeImg
          src={thumb}
          alt=""
          className="w-20 h-20 shrink-0 object-cover rounded-md border-2"
          style={{ borderColor: "var(--color-border)" }}
          loading="lazy"
        />
      ) : (
        <div
          className="w-20 h-20 shrink-0 rounded-md border-2"
          style={{ borderColor: "var(--color-border-soft)", backgroundColor: "var(--color-bg-muted)" }}
          aria-hidden
        />
      )}
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium mb-1 leading-snug">{article.title}</p>
        {article.summary && (
          <p className="text-sm leading-relaxed mb-2 line-clamp-2" style={{ color: "var(--color-ink-light)" }}>
            {article.summary}
          </p>
        )}
        <p className="text-xs" style={{ color: "var(--color-ink-muted)" }}>
          {article._subName} · {formatRelativeTime(article.publish_at)} · 阅读 {article.read_num} · 赞 {article.like_num}
        </p>
      </div>
    </div>
  );

  if (article.url) {
    return (
      <a
        href={article.url}
        target="_blank"
        rel="noopener noreferrer"
        className="block py-4 border-b last:border-b-0 hover:bg-black/[0.02] -mx-2 px-2"
        style={{ borderColor: "var(--color-border)" }}
      >
        {inner}
      </a>
    );
  }

  return (
    <div className="py-4 border-b last:border-b-0" style={{ borderColor: "var(--color-border)" }}>
      {inner}
    </div>
  );
}
