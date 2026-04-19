import { useState, useEffect, useMemo } from "react";
import { SafeImg } from "@/components/SafeImg";
import { useQuery } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { api, type Article } from "@/lib/api";
import { formatRelativeTime } from "@/lib/utils";

const PAGE_SIZE = 20;

export default function RSSFeedsPage() {
  const [globalCopied, setGlobalCopied] = useState(false);
  const [offset, setOffset] = useState(0);
  const [allArticles, setAllArticles] = useState<Article[]>([]);
  const [hasMore, setHasMore] = useState(true);

  const { data: me } = useQuery({
    queryKey: ["me"],
    queryFn: () => api.getMe(),
  });

  const { data: subscriptions } = useQuery({
    queryKey: ["subscriptions"],
    queryFn: () => api.getSubscriptions(),
    staleTime: 30_000,
  });

  const subMap = useMemo(() => {
    const map = new Map<string, string>();
    subscriptions?.forEach((s) => {
      map.set(s.book_id, s.alias || s.mp_name || "");
    });
    return map;
  }, [subscriptions]);

  const globalFeedUrl = me?.global_feed_url ?? "";

  const {
    data: pageArticles,
    isLoading,
    isFetching,
  } = useQuery({
    queryKey: ["global-articles", offset],
    queryFn: () => api.getGlobalArticles(PAGE_SIZE, offset),
    enabled: hasMore || offset === 0,
    staleTime: 60_000,
  });

  // Merge pages
  useEffect(() => {
    if (!pageArticles) return;
    if (pageArticles.length === 0) {
      setHasMore(false);
      return;
    }
    if (offset === 0) {
      setAllArticles(pageArticles);
    } else {
      setAllArticles((prev) => {
        const existingIds = new Set(prev.map((a) => a.review_id));
        const newOnes = pageArticles.filter((a) => !existingIds.has(a.review_id));
        return [...prev, ...newOnes];
      });
    }
    if (pageArticles.length < PAGE_SIZE) {
      setHasMore(false);
    }
  }, [pageArticles, offset]);

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

  return (
    <div className="page-enter max-w-2xl mx-auto">
      <header className="mb-6 md:mb-8">
        <h1 className="text-2xl md:text-4xl font-heading mb-1">文章流</h1>
        <p className="text-sm md:text-lg" style={{ color: "var(--color-ink-muted)" }}>
          全部订阅按发布时间合并
        </p>
      </header>

      {globalFeedUrl && (
        <div
          className="mb-6 md:mb-8 pb-4 md:pb-6 border-b-2"
          style={{ borderColor: "var(--color-border-soft)" }}
        >
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
              <button
                type="button"
                onClick={handleCopyGlobal}
                className="btn-primary text-sm px-3 py-1.5 rounded-full"
              >
                {globalCopied ? "已复制" : "复制"}
              </button>
              <a
                href={globalFeedUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="btn-secondary text-sm px-3 py-1.5 rounded-full"
              >
                打开
              </a>
            </div>
          </div>
        </div>
      )}

      {isLoading && allArticles.length === 0 ? (
        <div className="flex justify-center py-20">
          <Loader2
            className="w-5 h-5 animate-spin"
            style={{ color: "var(--color-ink-muted)" }}
          />
        </div>
      ) : allArticles.length === 0 ? (
        <p className="text-sm py-12" style={{ color: "var(--color-ink-muted)" }}>
          尚无文章，等待抓取或到订阅页手动刷新
        </p>
      ) : (
        <>
          <div
            className="border-t-2 mb-6"
            style={{ borderColor: "var(--color-border-soft)" }}
          >
            {allArticles.map((article) => (
              <FeedArticleRow
                key={`${article.id}-${article.review_id}`}
                article={article}
                subName={subMap.get(article.book_id) || ""}
              />
            ))}
          </div>

          {/* Load more */}
          <div className="flex justify-center py-6">
            {isFetching ? (
              <Loader2
                className="w-5 h-5 animate-spin"
                style={{ color: "var(--color-ink-muted)" }}
              />
            ) : hasMore ? (
              <button
                type="button"
                onClick={() => setOffset((prev) => prev + PAGE_SIZE)}
                className="text-sm px-5 py-2.5 border-2 rounded-full transition-colors"
                style={{ borderColor: "var(--color-border)" }}
              >
                加载更多
              </button>
            ) : (
              <p className="text-xs" style={{ color: "var(--color-ink-faint)" }}>
                已加载全部
              </p>
            )}
          </div>
        </>
      )}
    </div>
  );
}

function FeedArticleRow({
  article,
  subName,
}: {
  article: Article;
  subName: string;
}) {
  const thumb = article.cover_url?.trim();

  const inner = (
    <div className="flex gap-3 items-start min-w-0">
      {thumb ? (
        <SafeImg
          src={thumb}
          alt=""
          className="w-16 h-16 md:w-20 md:h-20 shrink-0 object-cover rounded-md border-2"
          style={{ borderColor: "var(--color-border)" }}
          loading="lazy"
        />
      ) : (
        <div
          className="w-16 h-16 md:w-20 md:h-20 shrink-0 rounded-md border-2"
          style={{
            borderColor: "var(--color-border-soft)",
            backgroundColor: "var(--color-bg-muted)",
          }}
          aria-hidden
        />
      )}
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium mb-1 leading-snug line-clamp-2">
          {article.title}
        </p>
        {article.summary && (
          <p
            className="text-xs md:text-sm leading-relaxed mb-2 line-clamp-2"
            style={{ color: "var(--color-ink-light)" }}
          >
            {article.summary}
          </p>
        )}
        <p className="text-xs" style={{ color: "var(--color-ink-muted)" }}>
          {subName ? `${subName} · ` : ""}
          {formatRelativeTime(article.publish_at)}
          {article.read_num > 0 ? ` · 阅读 ${article.read_num}` : ""}
          {article.like_num > 0 ? ` · 赞 ${article.like_num}` : ""}
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
    <div
      className="py-4 border-b last:border-b-0"
      style={{ borderColor: "var(--color-border)" }}
    >
      {inner}
    </div>
  );
}

