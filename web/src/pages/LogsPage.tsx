import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { Activity, CheckCircle2, XCircle, Clock, AlertTriangle } from "lucide-react";

function formatTime(ts: number) {
  return new Date(ts * 1000).toLocaleString("zh-CN");
}

function formatDuration(ms: number) {
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

export default function LogsPage() {
  const [offset, setOffset] = useState(0);
  const now = Math.floor(Date.now() / 1000);
  const since = now - 24 * 3600;

  const { data: statsData } = useQuery({
    queryKey: ["fetch-stats", since, now],
    queryFn: () => api.getFetchStats(since, now, 1800),
    refetchInterval: 60_000,
  });

  const { data: logsData } = useQuery({
    queryKey: ["fetch-logs", offset],
    queryFn: () => api.getFetchLogs(undefined, offset),
  });

  const stats = statsData?.stats ?? [];
  const failRate = statsData?.fail_rate ?? 0;
  const logs = logsData ?? [];

  const totalAttempts = stats.reduce((sum, s) => sum + s.total, 0);
  const totalSuccess = stats.reduce((sum, s) => sum + s.success, 0);
  const overallPct = totalAttempts > 0 ? (totalSuccess / totalAttempts) * 100 : 0;

  const chainLabel: Record<string, string> = {
    web: "微信读书网页端",
    mp: "公众号公开页",
    shareChapter: "App 接口",
  };

  return (
    <div className="page-enter max-w-5xl mx-auto">
      <header className="mb-8">
        <h1 className="text-3xl font-heading mb-1 flex items-center gap-2">
          <Activity className="h-6 w-6" />
          日志
        </h1>
        <p className="text-sm" style={{ color: "var(--color-ink-muted)" }}>
          正文抓取链路统计与详细记录
        </p>
      </header>

      {/* Overview cards */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-8">
        <div
          className="rounded-lg border p-4"
          style={{ backgroundColor: "var(--color-bg-card)", borderColor: "var(--color-border)" }}
        >
          <div className="flex items-center gap-2 mb-1 text-sm" style={{ color: "var(--color-ink-muted)" }}>
            <CheckCircle2 className="h-4 w-4" />
            今日成功率
          </div>
          <div className="text-2xl font-heading">{overallPct.toFixed(1)}%</div>
          <div className="text-xs mt-1" style={{ color: "var(--color-ink-faint)" }}>
            {totalSuccess} / {totalAttempts} 次成功
          </div>
        </div>

        <div
          className="rounded-lg border p-4"
          style={{ backgroundColor: "var(--color-bg-card)", borderColor: "var(--color-border)" }}
        >
          <div className="flex items-center gap-2 mb-1 text-sm" style={{ color: "var(--color-ink-muted)" }}>
            <Clock className="h-4 w-4" />
            近 30 分钟失败率
          </div>
          <div
            className="text-2xl font-heading"
            style={{ color: failRate > 80 ? "var(--color-danger)" : "var(--color-ink)" }}
          >
            {failRate >= 0 ? failRate.toFixed(1) + "%" : "--"}
          </div>
          {failRate > 80 && (
            <div className="text-xs mt-1 flex items-center gap-1" style={{ color: "var(--color-danger)" }}>
              <AlertTriangle className="h-3 w-3" />
              超过告警阈值
            </div>
          )}
        </div>

        <div
          className="rounded-lg border p-4"
          style={{ backgroundColor: "var(--color-bg-card)", borderColor: "var(--color-border)" }}
        >
          <div className="flex items-center gap-2 mb-1 text-sm" style={{ color: "var(--color-ink-muted)" }}>
            <Activity className="h-4 w-4" />
            活跃链路数
          </div>
          <div className="text-2xl font-heading">{stats.length}</div>
          <div className="text-xs mt-1" style={{ color: "var(--color-ink-faint)" }}>
            {stats.map((s) => chainLabel[s.chain] || s.chain).join("、")}
          </div>
        </div>
      </div>

      {/* Chain stats */}
      <section className="mb-10">
        <h2 className="text-xl font-heading mb-4">链路统计（最近 24 小时）</h2>
        {stats.length === 0 ? (
          <p className="text-sm" style={{ color: "var(--color-ink-muted)" }}>
            暂无数据
          </p>
        ) : (
          <div className="space-y-3">
            {stats.map((s) => {
              const label = chainLabel[s.chain] || s.chain;
              const pct = s.total > 0 ? (s.success / s.total) * 100 : 0;
              return (
                <div
                  key={s.chain}
                  className="flex items-center gap-4 rounded-lg border p-3"
                  style={{ borderColor: "var(--color-border)" }}
                >
                  <div className="w-28 shrink-0 text-sm font-medium">{label}</div>
                  <div className="flex-1">
                    <div className="h-2 rounded-full overflow-hidden" style={{ backgroundColor: "var(--color-bg-muted)" }}>
                      <div
                        className="h-full rounded-full transition-all"
                        style={{
                          width: `${pct}%`,
                          backgroundColor:
                            pct >= 80
                              ? "var(--color-success, #22c55e)"
                              : pct >= 50
                              ? "var(--color-warn, #f59e0b)"
                              : "var(--color-danger)",
                        }}
                      />
                    </div>
                  </div>
                  <div className="w-32 shrink-0 text-right text-xs" style={{ color: "var(--color-ink-muted)" }}>
                    {s.success} / {s.total} ({pct.toFixed(0)}%) · 均时 {formatDuration(s.avg_cost_ms)}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </section>

      {/* Logs table */}
      <section>
        <h2 className="text-xl font-heading mb-4">最近记录</h2>
        {logs.length === 0 ? (
          <p className="text-sm" style={{ color: "var(--color-ink-muted)" }}>
            暂无抓取记录
          </p>
        ) : (
          <>
            <div className="border rounded-lg overflow-hidden" style={{ borderColor: "var(--color-border)" }}>
              <table className="w-full text-sm">
                <thead>
                  <tr
                    className="text-left text-xs uppercase tracking-wider"
                    style={{ backgroundColor: "var(--color-bg-muted)", color: "var(--color-ink-muted)" }}
                  >
                    <th className="px-4 py-2 font-medium">时间</th>
                    <th className="px-4 py-2 font-medium">链路</th>
                    <th className="px-4 py-2 font-medium">结果</th>
                    <th className="px-4 py-2 font-medium">耗时</th>
                    <th className="px-4 py-2 font-medium">错误信息</th>
                  </tr>
                </thead>
                <tbody>
                  {logs.map((log) => (
                    <tr
                      key={log.id}
                      className="border-t"
                      style={{ borderColor: "var(--color-border)" }}
                    >
                      <td className="px-4 py-2 whitespace-nowrap">{formatTime(log.created_at)}</td>
                      <td className="px-4 py-2">{chainLabel[log.chain] || log.chain}</td>
                      <td className="px-4 py-2">
                        {log.success ? (
                          <span className="inline-flex items-center gap-1" style={{ color: "var(--color-success, #22c55e)" }}>
                            <CheckCircle2 className="h-3.5 w-3.5" />
                            成功
                          </span>
                        ) : (
                          <span className="inline-flex items-center gap-1" style={{ color: "var(--color-danger)" }}>
                            <XCircle className="h-3.5 w-3.5" />
                            失败
                          </span>
                        )}
                      </td>
                      <td className="px-4 py-2">{formatDuration(log.cost_ms)}</td>
                      <td
                        className="px-4 py-2 max-w-xs truncate"
                        title={log.error || ""}
                        style={{ color: "var(--color-ink-muted)" }}
                      >
                        {log.error || "—"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <div className="flex items-center justify-between mt-4">
              <button
                type="button"
                className="text-sm px-3 py-1.5 rounded border disabled:opacity-40"
                style={{ borderColor: "var(--color-border)" }}
                disabled={offset === 0}
                onClick={() => setOffset((p) => Math.max(0, p - 50))}
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
                disabled={logs.length < 50}
                onClick={() => setOffset((p) => p + 50)}
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
