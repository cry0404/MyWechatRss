import { Fragment, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, type FetchEvent } from "@/lib/api";
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  ChevronDown,
  ChevronUp,
  Clock,
  Copy,
  Filter,
  XCircle,
} from "lucide-react";

function formatTime(ts?: number) {
  if (!ts) return "--";
  return new Date(ts * 1000).toLocaleString("zh-CN");
}

function formatDurationMs(ms: number) {
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

function formatIntervalSec(sec?: number) {
  if (!sec || sec <= 0) return "首次记录";
  if (sec < 60) return `${sec}s`;
  const minutes = Math.floor(sec / 60);
  if (minutes < 60) return `${minutes}m ${sec % 60}s`;
  const hours = Math.floor(minutes / 60);
  const restMinutes = minutes % 60;
  if (hours < 24) return `${hours}h ${restMinutes}m`;
  const days = Math.floor(hours / 24);
  return `${days}d ${hours % 24}h`;
}

type FilterMode = "all" | "rate-limit" | "fail" | "success";

const chainLabel: Record<string, string> = {
  source: "订阅源列表",
  "web/mp/articles": "Web 文章列表",
  "app/book/articles": "App 文章列表",
  web: "微信读书网页端",
  mp: "公众号公开页",
  shareChapter: "App 接口",
};

function chainTone(chain: string) {
  switch (chain) {
    case "source":
      return "var(--color-accent, #2563eb)";
    case "web/mp/articles":
      return "var(--color-success, #22c55e)";
    case "app/book/articles":
      return "var(--color-danger)";
    case "web":
      return "var(--color-success, #22c55e)";
    case "mp":
      return "var(--color-warn, #f59e0b)";
    case "shareChapter":
      return "var(--color-danger)";
    default:
      return "var(--color-ink-muted)";
  }
}

function sourceName(event: FetchEvent) {
  return event.subscription_alias || event.mp_name || event.book_id || event.review_id || "--";
}

export default function LogsPage() {
  const [offset, setOffset] = useState(0);
  const [filter, setFilter] = useState<FilterMode>("all");
  const [expandedKey, setExpandedKey] = useState<string | null>(null);
  const rateLimitOnly = filter === "rate-limit";

  const { data: eventsData } = useQuery({
    queryKey: ["fetch-events", offset, rateLimitOnly],
    queryFn: () => api.getFetchEvents(offset, rateLimitOnly),
    refetchInterval: 30_000,
  });

  const { data: latestRateLimits } = useQuery({
    queryKey: ["fetch-events", "rate-limit-latest"],
    queryFn: () => api.getFetchEvents(0, true),
    refetchInterval: 30_000,
  });

  const events = eventsData ?? [];
  const visibleEvents =
    filter === "fail"
      ? events.filter((event) => !event.success)
      : filter === "success"
      ? events.filter((event) => event.success)
      : events;

  const successCount = events.filter((event) => event.success).length;
  const failCount = events.length - successCount;
  const rateLimitCount = events.filter((event) => event.error_code === "-2041").length;
  const latestRateLimit = latestRateLimits?.[0];
  const chainCounts = events.reduce<Record<string, number>>((acc, event) => {
    acc[event.chain] = (acc[event.chain] || 0) + 1;
    return acc;
  }, {});

  return (
    <div className="page-enter max-w-6xl mx-auto">
      <header className="mb-8">
        <h1 className="text-3xl font-heading mb-1 flex items-center gap-2">
          <Activity className="h-6 w-6" />
          日志
        </h1>
        <p className="text-sm" style={{ color: "var(--color-ink-muted)" }}>
          订阅源列表、正文抓取链路与 -2041 风控间隔
        </p>
      </header>

      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-8">
        <div className="rounded-lg border p-4" style={{ backgroundColor: "var(--color-bg-card)", borderColor: "var(--color-border)" }}>
          <div className="flex items-center gap-2 mb-1 text-sm" style={{ color: "var(--color-ink-muted)" }}>
            <Activity className="h-4 w-4" />
            当前页链路
          </div>
          <div className="text-2xl font-heading">{events.length}</div>
          <div className="text-xs mt-1" style={{ color: "var(--color-ink-faint)" }}>
            {Object.entries(chainCounts)
              .map(([chain, count]) => `${chainLabel[chain] || chain} ${count}`)
              .join("、") || "暂无数据"}
          </div>
        </div>

        <div className="rounded-lg border p-4" style={{ backgroundColor: "var(--color-bg-card)", borderColor: "var(--color-border)" }}>
          <div className="flex items-center gap-2 mb-1 text-sm" style={{ color: "var(--color-ink-muted)" }}>
            <AlertTriangle className="h-4 w-4" />
            最近 -2041
          </div>
          <div className="text-lg font-heading truncate" style={{ color: latestRateLimit ? "var(--color-danger)" : "var(--color-ink)" }}>
            {latestRateLimit ? sourceName(latestRateLimit) : "暂无"}
          </div>
          <div className="text-xs mt-1" style={{ color: "var(--color-ink-faint)" }}>
            {latestRateLimit ? formatTime(latestRateLimit.created_at) : "没有记录到列表风控"}
          </div>
        </div>

        <div className="rounded-lg border p-4" style={{ backgroundColor: "var(--color-bg-card)", borderColor: "var(--color-border)" }}>
          <div className="flex items-center gap-2 mb-1 text-sm" style={{ color: "var(--color-ink-muted)" }}>
            <Clock className="h-4 w-4" />
            -2041 间隔
          </div>
          <div className="text-2xl font-heading">
            {latestRateLimit ? formatIntervalSec(latestRateLimit.seconds_since_last_rate_limit) : "--"}
          </div>
          <div className="text-xs mt-1" style={{ color: "var(--color-ink-faint)" }}>
            当前页 {rateLimitCount} 条 -2041，失败 {failCount} 条，成功 {successCount} 条
          </div>
        </div>
      </div>

      <section>
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between mb-4">
          <h2 className="text-xl font-heading">链路记录</h2>
          <div className="flex flex-wrap items-center gap-2">
            <Filter className="h-4 w-4" style={{ color: "var(--color-ink-muted)" }} />
            {([
              { key: "all", label: `全部 (${events.length})` },
              { key: "rate-limit", label: "-2041" },
              { key: "fail", label: `失败 (${failCount})` },
              { key: "success", label: `成功 (${successCount})` },
            ] as { key: FilterMode; label: string }[]).map((item) => (
              <button
                key={item.key}
                type="button"
                onClick={() => {
                  setFilter(item.key);
                  setOffset(0);
                  setExpandedKey(null);
                }}
                className="text-xs px-2.5 py-1 rounded-md border transition-colors"
                style={{
                  borderColor: "var(--color-border)",
                  backgroundColor: filter === item.key ? "var(--color-bg-hover)" : "transparent",
                  color: filter === item.key ? "var(--color-ink)" : "var(--color-ink-muted)",
                }}
              >
                {item.label}
              </button>
            ))}
          </div>
        </div>

        {visibleEvents.length === 0 ? (
          <p className="text-sm" style={{ color: "var(--color-ink-muted)" }}>
            暂无符合条件的抓取记录
          </p>
        ) : (
          <>
            <div className="border rounded-lg overflow-x-auto" style={{ borderColor: "var(--color-border)" }}>
              <table className="w-full min-w-[980px] text-sm">
                <thead>
                  <tr className="text-left text-xs uppercase tracking-wider" style={{ backgroundColor: "var(--color-bg-muted)", color: "var(--color-ink-muted)" }}>
                    <th className="px-4 py-2 font-medium">时间</th>
                    <th className="px-4 py-2 font-medium">链路</th>
                    <th className="px-4 py-2 font-medium">订阅源 / 文章</th>
                    <th className="px-4 py-2 font-medium">结果</th>
                    <th className="px-4 py-2 font-medium">-2041 间隔</th>
                    <th className="px-4 py-2 font-medium">耗时 / 新增</th>
                    <th className="px-4 py-2 font-medium w-10"></th>
                  </tr>
                </thead>
                <tbody>
                  {visibleEvents.map((event) => {
                    const rowKey = `${event.event_type}-${event.id}-${event.created_at}`;
                    const isExpanded = expandedKey === rowKey;
                    const isRateLimited = event.error_code === "-2041";
                    const copyValue = event.review_id || event.book_id || "";
                    return (
                      <Fragment key={rowKey}>
                        <tr
                          className="border-t cursor-pointer hover:opacity-80"
                          style={{
                            borderColor: "var(--color-border)",
                            backgroundColor: isRateLimited ? "color-mix(in srgb, var(--color-danger) 9%, transparent)" : "transparent",
                          }}
                          onClick={() => setExpandedKey(isExpanded ? null : rowKey)}
                        >
                          <td className="px-4 py-2 whitespace-nowrap">{formatTime(event.created_at)}</td>
                          <td className="px-4 py-2">
                            <span className="inline-flex items-center gap-2">
                              <span className="h-2 w-2 rounded-full" style={{ backgroundColor: chainTone(event.chain) }} />
                              {chainLabel[event.chain] || event.chain}
                            </span>
                          </td>
                          <td className="px-4 py-2">
                            <div className="font-medium truncate max-w-[260px]">{sourceName(event)}</div>
                            <button
                              type="button"
                              className="inline-flex items-center gap-1 text-xs font-mono hover:underline"
                              style={{ color: "var(--color-ink-muted)" }}
                              onClick={(e) => {
                                e.stopPropagation();
                                if (copyValue) navigator.clipboard.writeText(copyValue);
                              }}
                              title="点击复制 ID"
                              disabled={!copyValue}
                            >
                              {(copyValue || "no-id").slice(0, 18)}
                              {copyValue.length > 18 ? "..." : ""}
                              {copyValue && <Copy className="h-3 w-3" />}
                            </button>
                          </td>
                          <td className="px-4 py-2">
                            {event.success ? (
                              <span className="inline-flex items-center gap-1" style={{ color: "var(--color-success, #22c55e)" }}>
                                <CheckCircle2 className="h-3.5 w-3.5" />
                                成功
                              </span>
                            ) : (
                              <span className="inline-flex items-center gap-1" style={{ color: isRateLimited ? "var(--color-danger)" : "var(--color-ink-muted)" }}>
                                <XCircle className="h-3.5 w-3.5" />
                                {event.error_code || "失败"}
                              </span>
                            )}
                          </td>
                          <td className="px-4 py-2">
                            {isRateLimited ? (
                              <div>
                                <div className="font-medium" style={{ color: "var(--color-danger)" }}>
                                  {formatIntervalSec(event.seconds_since_last_rate_limit)}
                                </div>
                                <div className="text-xs" style={{ color: "var(--color-ink-faint)" }}>
                                  上次 {formatTime(event.previous_rate_limit_at)}
                                </div>
                              </div>
                            ) : (
                              <span style={{ color: "var(--color-ink-faint)" }}>--</span>
                            )}
                          </td>
                          <td className="px-4 py-2">
                            <div>{formatDurationMs(event.cost_ms)}</div>
                            {event.event_type === "source" && (
                              <div className="text-xs" style={{ color: "var(--color-ink-faint)" }}>
                                新增 {event.new_count || 0} 篇
                              </div>
                            )}
                          </td>
                          <td className="px-4 py-2">
                            {event.error ? (
                              isExpanded ? (
                                <ChevronUp className="h-4 w-4" style={{ color: "var(--color-ink-muted)" }} />
                              ) : (
                                <ChevronDown className="h-4 w-4" style={{ color: "var(--color-ink-muted)" }} />
                              )
                            ) : null}
                          </td>
                        </tr>
                        {isExpanded && event.error && (
                          <tr style={{ backgroundColor: "var(--color-bg-muted)" }}>
                            <td colSpan={7} className="px-4 py-3">
                              <div className="text-xs font-medium mb-1" style={{ color: isRateLimited ? "var(--color-danger)" : "var(--color-ink-muted)" }}>
                                错误信息
                              </div>
                              <pre className="text-xs font-mono whitespace-pre-wrap break-all" style={{ color: "var(--color-ink-muted)" }}>
                                {event.error}
                              </pre>
                            </td>
                          </tr>
                        )}
                      </Fragment>
                    );
                  })}
                </tbody>
              </table>
            </div>

            <div className="flex items-center justify-between mt-4">
              <button
                type="button"
                className="text-sm px-3 py-1.5 rounded border disabled:opacity-40"
                style={{ borderColor: "var(--color-border)" }}
                disabled={offset === 0}
                onClick={() => setOffset((prev) => Math.max(0, prev - 50))}
              >
                上一页
              </button>
              <span className="text-xs" style={{ color: "var(--color-ink-muted)" }}>
                第 {Math.floor(offset / 50) + 1} 页
              </span>
              <button
                type="button"
                className="text-sm px-3 py-1.5 rounded border disabled:opacity-40"
                style={{ borderColor: "var(--color-border)" }}
                disabled={events.length < 50}
                onClick={() => setOffset((prev) => prev + 50)}
              >
                下一页
              </button>
            </div>
          </>
        )}
      </section>
    </div>
  );
}
