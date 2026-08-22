import { useMemo, useState, type ReactNode } from "react";
import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { Check, Loader2 } from "lucide-react";
import { useAuthStore } from "@/stores/authStore";
import { api } from "@/lib/api";
import {
  copyToClipboard,
  formatFutureTime,
  formatRelativeTime,
  isFeedOnboardingReady,
  markFeedOnboardingReady,
  truncateText,
} from "@/lib/utils";
import { SafeImg } from "@/components/SafeImg";

function SetupGuide({
  passwordReady,
  accountReady,
  subscriptionReady,
  feedReady,
  message,
  action,
}: {
  passwordReady: boolean;
  accountReady: boolean;
  subscriptionReady: boolean;
  feedReady: boolean;
  message: string;
  action: ReactNode;
}) {
  const steps = [
    { label: "修改初始默认密码", done: passwordReady },
    { label: "绑定微信读书账号", done: accountReady },
    { label: "添加第一个公众号", done: subscriptionReady },
    { label: "复制聚合 RSS 到阅读器", done: feedReady },
  ];

  return (
    <section
      className="rounded-xl border-2 bg-white p-5 sm:p-7"
      style={{ borderColor: "var(--color-border)" }}
      aria-labelledby="setup-title"
    >
      <p className="text-xs font-medium mb-2" style={{ color: "var(--color-secondary)" }}>
        首次设置
      </p>
      <h2 id="setup-title" className="text-2xl font-heading mb-2">
        只差几步即可开始使用
      </h2>
      <p className="text-sm leading-relaxed mb-6" style={{ color: "var(--color-ink-muted)" }}>
        {message}
      </p>

      <ol className="space-y-3 mb-7">
        {steps.map((step, index) => (
          <li key={step.label} className="flex items-center gap-3">
            <span
              className="h-7 w-7 shrink-0 rounded-full border-2 flex items-center justify-center text-xs font-semibold"
              style={{
                borderColor: step.done ? "var(--color-success)" : "var(--color-border-soft)",
                backgroundColor: step.done ? "var(--color-success-bg)" : "var(--color-bg-surface)",
                color: step.done ? "var(--color-success)" : "var(--color-ink-muted)",
              }}
              aria-hidden
            >
              {step.done ? <Check className="h-4 w-4" strokeWidth={2.5} /> : index + 1}
            </span>
            <span
              className="text-base"
              style={{ color: step.done ? "var(--color-ink-muted)" : "var(--color-ink)" }}
            >
              {step.label}
            </span>
          </li>
        ))}
      </ol>

      {action}
    </section>
  );
}

export default function DashboardPage() {
  const token = useAuthStore((s) => s.token);
  const [feedReady, setFeedReady] = useState(isFeedOnboardingReady);
  const [isCopyingFeed, setIsCopyingFeed] = useState(false);
  const [copyFeedError, setCopyFeedError] = useState("");
  const meQuery = useQuery({
    queryKey: ["me"],
    queryFn: () => api.getMe(),
    enabled: !!token,
  });
  const me = meQuery.data;
  const displayName = me?.username ?? "用户";

  const today = useMemo(() => {
    return new Date().toLocaleDateString("zh-CN", {
      year: "numeric",
      month: "long",
      day: "numeric",
      weekday: "long",
    });
  }, []);

  const subscriptionsQuery = useQuery({
    queryKey: ["subscriptions"],
    queryFn: api.getSubscriptions,
  });
  const accountsQuery = useQuery({
    queryKey: ["accounts"],
    queryFn: api.getAccounts,
  });
  const subscriptions = subscriptionsQuery.data ?? [];
  const accounts = accountsQuery.data ?? [];

  const { data: globalArticles = [] } = useQuery({
    queryKey: ["global-articles", 0],
    queryFn: () => api.getGlobalArticles(10, 0),
    staleTime: 60_000,
  });

  const todayStart = useMemo(() => {
    const start = new Date();
    start.setHours(0, 0, 0, 0);
    return Math.floor(start.getTime() / 1000);
  }, []);
  const articleCountQuery = useQuery({
    queryKey: ["article-count", todayStart],
    queryFn: () => api.getArticleCount(todayStart),
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

  const todayArticleCount = articleCountQuery.data?.count;
  const activeAccountCount = accounts.filter((a) => a.status === "active").length;
  const enabledSubscriptions = subscriptions.filter((sub) => !sub.disabled);
  const lastSuccessfulFetch = enabledSubscriptions.reduce(
    (latest, sub) => Math.max(latest, sub.last_fetch_at ?? 0),
    0
  );
  const nextScheduledFetch = enabledSubscriptions.reduce((next, sub) => {
    const candidate =
      sub.next_fetch_after && sub.next_fetch_after > 0
        ? sub.next_fetch_after
        : sub.last_fetch_at
          ? sub.last_fetch_at + sub.fetch_interval_sec
          : 0;
    if (!candidate) return next;
    return next === 0 ? candidate : Math.min(next, candidate);
  }, 0);
  const hasBoundAccount = accounts.length > 0;
  const hasSubscription = subscriptions.length > 0;
  const passwordReady = me ? !me.must_change_password : false;
  const setupLoading = meQuery.isLoading || accountsQuery.isLoading || subscriptionsQuery.isLoading;
  const setupError = meQuery.isError || accountsQuery.isError || subscriptionsQuery.isError;
  const showSetup = !setupLoading && !setupError && (!passwordReady || !hasBoundAccount || !hasSubscription || !feedReady);

  const handleCopyGlobalFeed = async () => {
    if (!me?.global_feed_url || isCopyingFeed) return;
    setCopyFeedError("");
    setIsCopyingFeed(true);
    const copied = await copyToClipboard(me.global_feed_url);
    setIsCopyingFeed(false);
    if (!copied) {
      setCopyFeedError("复制失败，请到文章流页面手动复制 RSS 地址。");
      return;
    }
    markFeedOnboardingReady();
    setFeedReady(true);
  };

  let setupMessage = "先修改初始默认密码，保护这个管理页面。";
  let setupAction: ReactNode = (
    <Link to="/settings#password" className="btn-primary text-base px-5 py-2.5">
      修改密码
    </Link>
  );
  if (passwordReady && !hasBoundAccount) {
    setupMessage = "先绑定微信读书账号，之后才能搜索公众号和抓取文章。";
    setupAction = (
      <Link to="/accounts" className="btn-primary text-base px-5 py-2.5">
        绑定微信读书
      </Link>
    );
  } else if (passwordReady && hasBoundAccount && activeAccountCount === 0) {
    setupMessage = "已绑定的账号当前不可用，请先查看账号状态，恢复后再添加订阅。";
    setupAction = (
      <Link to="/accounts" className="btn-primary text-base px-5 py-2.5">
        查看账号状态
      </Link>
    );
  } else if (passwordReady && hasBoundAccount && !hasSubscription) {
    setupMessage = "账号已经就绪，现在添加第一个公众号。";
    setupAction = (
      <Link to="/subscriptions" className="btn-primary text-base px-5 py-2.5">
        添加第一个订阅
      </Link>
    );
  } else if (passwordReady && hasSubscription && !feedReady) {
    setupMessage = "订阅已经就绪，复制聚合 RSS 后即可添加到你的阅读器。";
    setupAction = (
      <div>
        <button
          type="button"
          onClick={handleCopyGlobalFeed}
          disabled={!me?.global_feed_url || isCopyingFeed}
          className="btn-primary text-base px-5 py-2.5 disabled:opacity-50"
        >
          {isCopyingFeed ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
          {isCopyingFeed ? "复制中…" : "复制聚合 RSS"}
        </button>
        {copyFeedError ? (
          <p className="text-sm mt-3" style={{ color: "var(--color-danger)" }} role="alert">
            {copyFeedError}
          </p>
        ) : null}
      </div>
    );
  }

  return (
    <div className="page-enter max-w-3xl mx-auto">
      <header className="mb-10">
        <h1 className="text-3xl md:text-4xl font-heading mb-1">欢迎，{displayName}</h1>
        <p className="text-lg" style={{ color: "var(--color-ink-muted)" }}>
          {today}
        </p>
      </header>

      {setupLoading && (
        <div className="flex items-center justify-center py-20" aria-label="正在加载设置状态">
          <Loader2 className="h-6 w-6 animate-spin" style={{ color: "var(--color-ink-muted)" }} />
        </div>
      )}

      {setupError && (
        <section
          className="rounded-xl border-2 bg-white p-6"
          style={{ borderColor: "var(--color-danger)" }}
          role="alert"
        >
          <h2 className="text-xl font-heading mb-2">暂时无法加载服务状态</h2>
          <p className="text-sm mb-4" style={{ color: "var(--color-ink-muted)" }}>
            请检查服务连接后重试。
          </p>
          <button type="button" onClick={() => window.location.reload()} className="btn-primary text-base px-5 py-2.5">
            重新加载
          </button>
        </section>
      )}

      {showSetup && (
        <SetupGuide
          passwordReady={passwordReady}
          accountReady={hasBoundAccount}
          subscriptionReady={hasSubscription}
          feedReady={feedReady}
          message={setupMessage}
          action={setupAction}
        />
      )}

      {!setupLoading && !setupError && !showSetup && (
        <>

      <section className="mb-10 grid gap-3 sm:grid-cols-2" aria-label="服务状态">
        <div className="rounded-xl border-2 bg-white p-4" style={{ borderColor: "var(--color-border-soft)" }}>
          <p className="text-xs mb-1" style={{ color: "var(--color-ink-muted)" }}>订阅与账号</p>
          <p className="text-lg font-medium">
            {subscriptions.length} 个订阅 · {activeAccountCount}/{accounts.length} 个账号可用
          </p>
          <p className="text-xs mt-2" style={{ color: "var(--color-ink-muted)" }}>
            今日发布 {articleCountQuery.isError ? "暂不可用" : todayArticleCount === undefined ? "加载中…" : `${todayArticleCount} 篇文章`}
          </p>
        </div>
        <div className="rounded-xl border-2 bg-white p-4" style={{ borderColor: "var(--color-border-soft)" }}>
          <p className="text-xs mb-1" style={{ color: "var(--color-ink-muted)" }}>自动抓取</p>
          <p className="text-sm font-medium">
            {lastSuccessfulFetch ? `最近成功 ${formatRelativeTime(lastSuccessfulFetch)}` : "尚未完成首次抓取"}
          </p>
          <p className="text-xs mt-2" style={{ color: "var(--color-ink-muted)" }}>
            {nextScheduledFetch
              ? nextScheduledFetch <= Math.floor(Date.now() / 1000)
                ? "下一轮抓取即将开始"
                : `下一轮 ${formatFutureTime(nextScheduledFetch)}`
              : "暂无自动抓取计划"}
          </p>
        </div>
      </section>

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
        </>
      )}
    </div>
  );
}
