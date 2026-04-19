import { useMemo } from "react";
import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useAuthStore } from "@/stores/authStore";
import { api } from "@/lib/api";
import { truncateText } from "@/lib/utils";
import { SafeImg } from "@/components/SafeImg";

const ONE_DAY_SEC = 24 * 60 * 60;

export default function DashboardPage() {
  const token = useAuthStore((s) => s.token);
  const { data: me } = useQuery({
    queryKey: ["me"],
    queryFn: () => api.getMe(),
    enabled: !!token,
  });
  const displayName = me?.username ?? "用户";

  const today = useMemo(() => {
    return new Date().toLocaleDateString("zh-CN", {
      year: "numeric",
      month: "long",
      day: "numeric",
      weekday: "long",
    });
  }, []);

  const { data: subscriptions = [] } = useQuery({
    queryKey: ["subscriptions"],
    queryFn: api.getSubscriptions,
  });
  const { data: accounts = [] } = useQuery({
    queryKey: ["accounts"],
    queryFn: api.getAccounts,
  });

  const { data: globalArticles = [] } = useQuery({
    queryKey: ["global-articles", 0],
    queryFn: () => api.getGlobalArticles(10, 0),
    staleTime: 60_000,
  });

  const subMap = useMemo(() => {
    const map = new Map<string, string>();
    subscriptions.forEach((s) => {
      map.set(s.book_id, s.alias || s.mp_name || "");
    });
    return map;
  }, [subscriptions]);

  const recentArticles = useMemo(() => {
    return globalArticles.slice(0, 5).map((a) => ({
      ...a,
      mp_name: subMap.get(a.book_id) || "",
    }));
  }, [globalArticles, subMap]);

  const nowSec = Math.floor(Date.now() / 1000);
  const todayArticleCount = recentArticles.filter((a) => nowSec - a.publish_at <= ONE_DAY_SEC).length;
  const activeAccountCount = accounts.filter((a) => a.status === "active").length;

  return (
    <div className="page-enter max-w-3xl mx-auto">
      <header className="mb-10">
        <h1 className="text-3xl md:text-4xl font-heading mb-1">欢迎，{displayName}</h1>
        <p className="text-lg" style={{ color: "var(--color-ink-muted)" }}>
          {today}
        </p>
      </header>

      <p className="text-xl mb-10 leading-relaxed" style={{ color: "var(--color-ink-light)" }}>
        订阅 {subscriptions.length} · 绑定账号 {accounts.length} · 可用 {activeAccountCount} · 今日文章约 {todayArticleCount}
      </p>

      <section className="mb-10">
        <div className="flex items-baseline justify-between mb-4">
          <h2 className="text-2xl font-heading">最近文章</h2>
          <Link to="/subscriptions" className="text-xs" style={{ color: "var(--color-ink-muted)" }}>
            全部订阅
          </Link>
        </div>

        <div className="border-t" style={{ borderColor: "var(--color-border)" }}>
          {recentArticles.length === 0 ? (
            <p className="py-8 text-sm" style={{ color: "var(--color-ink-muted)" }}>
              暂无文章。先到{" "}
              <Link to="/subscriptions" className="underline" style={{ color: "var(--color-ink)" }}>
                订阅
              </Link>{" "}
              添加公众号。
            </p>
          ) : (
            recentArticles.map((article) => {
              const hasURL = Boolean(article.url);
              const inner = (
                <>
                  {article.cover_url ? (
                    <SafeImg src={article.cover_url} alt="" className="w-11 h-11 rounded object-cover shrink-0" />
                  ) : (
                    <div className="w-11 h-11 rounded shrink-0" style={{ backgroundColor: "var(--color-bg-muted)" }} />
                  )}
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium truncate">{truncateText(article.title, 48)}</p>
                    <p className="text-xs mt-0.5 truncate" style={{ color: "var(--color-ink-muted)" }}>
                      {article.mp_name}
                    </p>
                  </div>
                </>
              );
              const rowClass = "flex items-center gap-3 py-3 border-b";
              const style = { borderColor: "var(--color-border)" };
              return hasURL ? (
                <a
                  key={article.id}
                  href={article.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className={rowClass}
                  style={style}
                >
                  {inner}
                </a>
              ) : (
                <div key={article.id} className={rowClass} style={style}>
                  {inner}
                </div>
              );
            })
          )}
        </div>
      </section>

      <section>
        <h2 className="text-2xl font-heading mb-3">快捷</h2>
        <ul className="space-y-2 text-xl">
          <li>
            <Link to="/subscriptions" className="underline underline-offset-2" style={{ color: "var(--color-ink)" }}>
              管理订阅
            </Link>
            <span className="text-xs ml-2" style={{ color: "var(--color-ink-muted)" }}>
              搜索并添加公众号
            </span>
          </li>
          <li>
            <Link to="/accounts" className="underline underline-offset-2" style={{ color: "var(--color-ink)" }}>
              账号与扫码
            </Link>
            <span className="text-xs ml-2" style={{ color: "var(--color-ink-muted)" }}>
              绑定微信读书
            </span>
          </li>
          <li>
            <Link to="/feeds" className="underline underline-offset-2" style={{ color: "var(--color-ink)" }}>
              聚合文章流
            </Link>
            <span className="text-xs ml-2" style={{ color: "var(--color-ink-muted)" }}>
              按时间浏览
            </span>
          </li>
        </ul>
      </section>
    </div>
  );
}
