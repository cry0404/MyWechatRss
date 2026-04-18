import { useState, useMemo } from "react";
import { Link } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Loader2,
  Plus,
  Download,
  SlidersHorizontal,
  ListFilter,
  MoreHorizontal,
} from "lucide-react";
import { api, type SearchResult, type Subscription } from "@/lib/api";
import { SafeImg } from "@/components/SafeImg";
import { useAlertStore } from "@/stores/alertStore";
import { toUserMessage } from "@/lib/userMessage";
import { formatRelativeTime } from "@/lib/utils";
import { formatFetchWindowLine, formatInterval } from "@/lib/schedule";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { SubscriptionScheduleModal } from "@/components/SubscriptionScheduleModal";
import { ActionMenu } from "@/components/ActionMenu";
import { ModalPortal } from "@/components/ModalPortal";

function StatMetric({
  label,
  value,
  dotColor,
}: {
  label: string;
  value: string | number;
  dotColor: string;
}) {
  return (
    <div className="stat-metric-card">
      <div className="flex items-start justify-between gap-2">
        <span className="text-sm font-heading" style={{ color: "var(--color-ink-muted)" }}>
          {label}
        </span>
        <span className="h-2 w-2 shrink-0 mt-0.5 rounded-sm" style={{ backgroundColor: dotColor }} />
      </div>
      <span className="text-3xl font-heading tabular-nums" style={{ color: "var(--color-ink)" }}>
        {value}
      </span>
    </div>
  );
}

function StatusPill({
  disabled,
  onClick,
  isPending,
}: {
  disabled: boolean;
  onClick?: () => void;
  isPending?: boolean;
}) {
  if (disabled) {
    return (
      <button
        type="button"
        onClick={onClick}
        disabled={isPending}
        className="inline-flex items-center gap-1.5 px-2.5 py-1 text-sm rounded-full border transition-colors hover:opacity-80 disabled:opacity-50"
        style={{ backgroundColor: "var(--color-bg-muted)", color: "var(--color-ink-muted)", borderColor: "var(--color-border-soft)" }}
      >
        <span className="h-1.5 w-1.5 rounded-sm" style={{ backgroundColor: "var(--color-ink-faint)" }} />
        已停用
      </button>
    );
  }
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={isPending}
      className="inline-flex items-center gap-1.5 px-2.5 py-1 text-sm rounded-full border transition-colors hover:opacity-80 disabled:opacity-50"
      style={{ backgroundColor: "var(--color-success-bg)", color: "var(--color-success)", borderColor: "var(--color-success)" }}
    >
      <span className="h-1.5 w-1.5 rounded-sm" style={{ backgroundColor: "var(--color-success)" }} />
      启用
    </button>
  );
}

function Avatar({ name, src }: { name: string; src?: string }) {
  const initials = name
    .split(/\s+/)
    .map((w) => w[0])
    .join("")
    .slice(0, 2)
    .toUpperCase();
  const hues = [
    ["var(--color-secondary-pale)", "var(--color-secondary)"],
    ["#fff4d6", "#b8860b"],
    ["var(--color-success-pale)", "var(--color-success)"],
    ["#fde8f0", "#a63d6d"],
    ["#e8f0ff", "#2d5da1"],
  ];
  const idx = name.split("").reduce((a, c) => a + c.charCodeAt(0), 0) % hues.length;
  const [bg, fg] = hues[idx];
  if (src) {
    return <SafeImg src={src} alt="" className="h-10 w-10 object-cover shrink-0 rounded-md border-2" style={{ borderColor: "var(--color-border)" }} />;
  }
  return (
    <div className="h-10 w-10 flex items-center justify-center text-sm font-heading shrink-0 rounded-md border-2" style={{ backgroundColor: bg, color: fg, borderColor: "var(--color-border)" }}>
      {initials}
    </div>
  );
}

function RowActions({
  sub,
  copied,
  onCopy,
  onSchedule,
}: {
  sub: Subscription;
  copied: boolean;
  onCopy: () => void;
  onSchedule: () => void;
}) {
  const [open, setOpen] = useState(false);
  const itemClass =
    "flex w-full items-center px-3 py-2 text-lg transition-colors hover:bg-black/[0.04] outline-none focus-visible:bg-black/[0.06]";

  return (
    <ActionMenu
      open={open}
      onOpenChange={setOpen}
      align="end"
      trigger={
        <button
          type="button"
          onClick={() => setOpen((v) => !v)}
          aria-expanded={open}
          aria-haspopup="menu"
          className="rounded-md p-1.5 transition-colors duration-100 hover:bg-black/[0.06] active:bg-black/[0.08]"
          style={{ color: "var(--color-ink-muted)" }}
          aria-label="More actions"
        >
          <MoreHorizontal className="h-4 w-4" strokeWidth={2.5} />
        </button>
      }
    >
      <Link
        to={`/subscriptions/${sub.id}`}
        role="menuitem"
        className={itemClass}
        style={{ color: "var(--color-ink-light)" }}
        onClick={() => setOpen(false)}
      >
        查看文章
      </Link>
      <button
        type="button"
        role="menuitem"
        className={itemClass}
        style={{ color: "var(--color-ink-light)" }}
        onClick={() => {
          onSchedule();
          setOpen(false);
        }}
      >
        抓取计划
      </button>
      <button
        type="button"
        role="menuitem"
        className={itemClass}
        style={{ color: "var(--color-ink-light)" }}
        onClick={() => {
          onCopy();
          setOpen(false);
        }}
      >
        {copied ? "已复制 RSS" : "复制 RSS"}
      </button>
    </ActionMenu>
  );
}

function AddSubscriptionModal({ onClose }: { onClose: () => void }) {
  const showAlert = useAlertStore((s) => s.show);
  const [query, setQuery] = useState("");
  const [inputValue, setInputValue] = useState("");
  const [selected, setSelected] = useState<SearchResult | null>(null);
  const [alias, setAlias] = useState("");
  const queryClient = useQueryClient();

  const searchQuery = useQuery({
    queryKey: ["search", query],
    queryFn: () => api.search(query),
    enabled: query.length >= 2,
    staleTime: 60_000,
  });

  const createMutation = useMutation({
    mutationFn: async (data: { book_id: string; alias: string }) => {
      const sub = await api.createSubscription(data.book_id, data.alias);
      await api.refreshSubscription(sub.id);
      return sub;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["subscriptions"] });
      onClose();
    },
    onError: (err) => showAlert(toUserMessage(err)),
  });

  const handleConfirm = () => {
    if (!selected || !alias.trim()) return;
    createMutation.mutate({ book_id: selected.book_id, alias: alias.trim() });
  };

  return (
    <ModalPortal>
      <div className="fixed inset-0 z-[1000] flex items-start justify-center pt-[12vh] px-4">
        <div className="absolute inset-0 z-0 bg-black/45" onClick={onClose} aria-hidden />
        <div
          className="relative z-[1] w-full max-w-lg border-2 bg-white overflow-hidden rounded-xl"
          style={{ borderColor: "var(--color-border)" }}
          onClick={(e) => e.stopPropagation()}
        >
        <div className="flex items-center justify-between px-5 py-4 border-b-2" style={{ borderColor: "var(--color-border-soft)" }}>
          <h3 className="text-xl font-heading">{selected ? "确认订阅" : "添加订阅"}</h3>
          <button type="button" onClick={onClose} className="text-xs" style={{ color: "var(--color-ink-muted)" }}>
            关闭
          </button>
        </div>

        <div className="px-5 py-5">
          {!selected ? (
            <>
              <input
                type="text"
                placeholder="搜索公众号名称，回车"
                value={inputValue}
                onChange={(e) => setInputValue(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") setQuery(inputValue.trim());
                }}
                className="input-search-pill text-lg mb-4"
                autoFocus
              />

              {searchQuery.isLoading && (
                <div className="flex justify-center py-8">
                  <Loader2 className="h-5 w-5 animate-spin" style={{ color: "var(--color-ink-muted)" }} />
                </div>
              )}

              {searchQuery.data && searchQuery.data.length === 0 && query.length >= 2 && (
                <p className="text-center py-8 text-sm" style={{ color: "var(--color-ink-muted)" }}>
                  未找到
                </p>
              )}

              {searchQuery.data && searchQuery.data.length > 0 && (
                <div className="max-h-72 overflow-y-auto rounded-md border-2" style={{ borderColor: "var(--color-border-soft)" }}>
                  {searchQuery.data.map((result) => (
                    <button
                      type="button"
                      key={result.book_id}
                      className="w-full flex items-center gap-3 p-3 text-left border-b-2 last:border-b-0 hover:bg-black/[0.02]"
                      style={{ borderColor: "var(--color-border-soft)" }}
                      onClick={() => {
                        setSelected(result);
                        setAlias(result.title);
                      }}
                    >
                      <SafeImg src={result.cover} alt="" className="h-10 w-10 object-cover shrink-0 rounded-md border-2" style={{ borderColor: "var(--color-border)" }} />
                      <div className="min-w-0 flex-1">
                        <p className="text-lg font-heading truncate">{result.title}</p>
                        <p className="text-xs truncate" style={{ color: "var(--color-ink-muted)" }}>
                          {result.author}
                        </p>
                      </div>
                    </button>
                  ))}
                </div>
              )}
            </>
          ) : (
            <div className="space-y-5">
              <div className="flex items-center gap-3 p-3 rounded-md border-2" style={{ backgroundColor: "var(--color-postit)", borderColor: "var(--color-border-soft)" }}>
                <SafeImg src={selected.cover} alt="" className="h-12 w-12 object-cover shrink-0 rounded-md border-2" style={{ borderColor: "var(--color-border)" }} />
                <div>
                  <p className="text-lg font-heading">{selected.title}</p>
                  <p className="text-xs" style={{ color: "var(--color-ink-muted)" }}>
                    {selected.author}
                  </p>
                </div>
              </div>

              <div>
                <label className="block text-xs font-medium mb-1.5" style={{ color: "var(--color-ink-muted)" }}>
                  显示名称
                </label>
                <input
                  type="text"
                  value={alias}
                  onChange={(e) => setAlias(e.target.value)}
                  className="input-editorial text-lg"
                  autoFocus
                />
              </div>

              <div className="flex gap-2">
                <button type="button" onClick={() => setSelected(null)} className="btn-secondary text-lg flex-1 py-2.5">
                  返回
                </button>
                <button
                  type="button"
                  onClick={handleConfirm}
                  disabled={!alias.trim() || createMutation.isPending}
                  className="btn-primary text-lg flex-1 py-2.5 disabled:opacity-50"
                >
                  {createMutation.isPending ? (
                    <span className="flex items-center justify-center gap-2">
                      <Loader2 className="h-4 w-4 animate-spin" />
                      抓取中…
                    </span>
                  ) : (
                    "添加"
                  )}
                </button>
              </div>
            </div>
          )}
        </div>
        </div>
      </div>
    </ModalPortal>
  );
}

export default function SubscriptionsPage() {
  const showAlert = useAlertStore((s) => s.show);
  const [scheduleFor, setScheduleFor] = useState<Subscription | null>(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [copiedId, setCopiedId] = useState<number | null>(null);
  const [searchQuery, setSearchQuery] = useState("");
  const [sortBy, setSortBy] = useState<"recent" | "name">("recent");
  const [pendingDelete, setPendingDelete] = useState<Subscription | null>(null);
  const queryClient = useQueryClient();

  const { data: subscriptions, isLoading } = useQuery({
    queryKey: ["subscriptions"],
    queryFn: () => api.getSubscriptions(),
  });

  const { data: accounts = [] } = useQuery({
    queryKey: ["accounts"],
    queryFn: () => api.getAccounts(),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: number) => api.deleteSubscription(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["subscriptions"] });
    },
    onError: (err) => showAlert(toUserMessage(err)),
  });

  const toggleMutation = useMutation({
    mutationFn: ({ id, disabled }: { id: number; disabled: boolean }) =>
      api.updateSubscription(id, { disabled }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["subscriptions"] });
    },
    onError: (err) => showAlert(toUserMessage(err)),
  });

  const handleCopyFeed = async (sub: Subscription) => {
    const url = `${window.location.origin}/rss/${sub.feed_id}`;
    try {
      await navigator.clipboard.writeText(url);
      setCopiedId(sub.id);
      setTimeout(() => setCopiedId(null), 2000);
    } catch {
      /* ignore */
    }
  };

  const exportJSON = () => {
    if (!subscriptions?.length) return;
    const blob = new Blob([JSON.stringify(subscriptions, null, 2)], { type: "application/json" });
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = `wechatread-subscriptions-${Date.now()}.json`;
    a.click();
    URL.revokeObjectURL(a.href);
  };

  const filtered = useMemo(() => {
    if (!subscriptions) return [];
    let result = [...subscriptions];
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      result = result.filter(
        (s) =>
          s.alias.toLowerCase().includes(q) ||
          s.mp_name.toLowerCase().includes(q)
      );
    }
    if (sortBy === "name") {
      result.sort((a, b) => a.alias.localeCompare(b.alias, "zh-CN"));
    } else {
      result.sort((a, b) => b.created_at - a.created_at);
    }
    return result;
  }, [subscriptions, searchQuery, sortBy]);

  const stats = useMemo(() => {
    const total = subscriptions?.length ?? 0;
    const active = subscriptions?.filter((s) => !s.disabled).length ?? 0;
    const disabled = subscriptions?.filter((s) => s.disabled).length ?? 0;
    const wereadOk = accounts.filter((a) => a.status === "active").length;
    return { total, active, disabled, wereadOk };
  }, [subscriptions, accounts]);

  return (
    <div className="page-enter w-full">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-8">
        <h1 className="text-4xl md:text-5xl font-heading" style={{ color: "var(--color-ink)" }}>
          订阅
        </h1>
        <div className="flex flex-wrap items-center gap-2">
          <button
            type="button"
            onClick={exportJSON}
            disabled={!subscriptions?.length}
            className="btn-secondary rounded-full text-base px-4 py-2 disabled:opacity-40"
          >
            <Download className="h-4 w-4 opacity-80" strokeWidth={2.5} />
            导出
          </button>
          <button
            type="button"
            className="p-2 rounded-full border-2 transition-colors opacity-40 cursor-not-allowed"
            style={{ borderColor: "var(--color-border-soft)" }}
            title="筛选（即将支持）"
            disabled
          >
            <SlidersHorizontal className="h-4 w-4" strokeWidth={2.5} />
          </button>
          <button
            type="button"
            className="p-2 rounded-full border-2 transition-colors opacity-40 cursor-not-allowed"
            style={{ borderColor: "var(--color-border-soft)" }}
            title="视图（即将支持）"
            disabled
          >
            <ListFilter className="h-4 w-4" strokeWidth={2.5} />
          </button>
          <button
            type="button"
            onClick={() => setModalOpen(true)}
            className="btn-primary rounded-full text-base px-5 py-2.5 gap-1.5"
          >
            <Plus className="h-4 w-4" strokeWidth={2.5} />
            添加
          </button>
        </div>
      </div>

      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
        <StatMetric label="订阅总数" value={stats.total} dotColor="var(--color-stat-gray)" />
        <StatMetric label="已启用" value={stats.active} dotColor="var(--color-stat-purple)" />
        <StatMetric label="已停用" value={stats.disabled} dotColor="var(--color-stat-orange)" />
        <StatMetric label="可用读书账号" value={stats.wereadOk} dotColor="var(--color-stat-green)" />
      </div>

      <div className="panel-elevated">
        <div
          className="flex flex-col sm:flex-row sm:items-center gap-3 px-4 sm:px-5 py-4 border-b-2"
          style={{ borderColor: "var(--color-border-soft)" }}
        >
          <input
            type="search"
            placeholder="搜索订阅名称…"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="input-search-pill text-lg flex-1 min-w-0 max-w-md"
          />
          <select
            value={sortBy}
            onChange={(e) => setSortBy(e.target.value as "recent" | "name")}
            className="text-base font-heading px-3 py-2 rounded-full border-2 outline-none cursor-pointer sm:ml-auto"
            style={{
              borderColor: "var(--color-border)",
              backgroundColor: "var(--color-bg-surface)",
              color: "var(--color-ink-muted)",
            }}
          >
            <option value="recent">排序：最近添加</option>
            <option value="name">排序：名称</option>
          </select>
        </div>

        {isLoading ? (
          <div className="flex justify-center py-20">
            <Loader2 className="h-5 w-5 animate-spin" style={{ color: "var(--color-ink-muted)" }} />
          </div>
        ) : !subscriptions || subscriptions.length === 0 ? (
          <div className="py-16 px-5 text-center">
            <p className="text-xl font-heading mb-1">还没有订阅</p>
            <p className="text-lg mb-5" style={{ color: "var(--color-ink-muted)" }}>
              添加公众号后开始抓取文章
            </p>
            <button type="button" onClick={() => setModalOpen(true)} className="btn-primary rounded-full text-base px-5 py-2.5 gap-1.5">
              <Plus className="h-4 w-4" strokeWidth={2.5} />
              添加
            </button>
          </div>
        ) : (
          <>
            <div
              className="hidden sm:grid grid-cols-12 gap-4 px-5 py-3 text-sm font-heading border-b-2"
              style={{ color: "var(--color-ink-faint)", borderColor: "var(--color-border-soft)" }}
            >
              <div className="col-span-4">账号</div>
              <div className="col-span-2">状态</div>
              <div className="col-span-2">上次抓取</div>
              <div className="col-span-2">抓取间隔</div>
              <div className="col-span-2 text-right">操作</div>
            </div>
            <div className="divide-y-2 divide-dashed" style={{ borderColor: "var(--color-border-soft)" }}>
              {filtered.map((sub) => (
                <div
                  key={sub.id}
                  className="grid grid-cols-1 sm:grid-cols-12 gap-3 sm:gap-4 px-4 sm:px-5 py-4 items-center transition-colors duration-100 hover:bg-black/[0.03]"
                >
                  <div className="col-span-4 flex items-center gap-3 min-w-0">
                    <Link to={`/subscriptions/${sub.id}`} className="flex items-center gap-3 min-w-0 group">
                      <Avatar name={sub.alias} src={sub.cover_url} />
                      <div className="min-w-0">
                        <p className="text-lg font-heading truncate group-hover:underline">{sub.alias}</p>
                        <p className="text-xs truncate" style={{ color: "var(--color-ink-muted)" }}>
                          @{sub.mp_name}
                        </p>
                      </div>
                    </Link>
                  </div>
                  <div className="col-span-2">
                    <StatusPill
                      disabled={sub.disabled}
                      onClick={() => toggleMutation.mutate({ id: sub.id, disabled: !sub.disabled })}
                      isPending={toggleMutation.isPending && toggleMutation.variables?.id === sub.id}
                    />
                  </div>
                  <div className="col-span-2 text-lg" style={{ color: "var(--color-ink-light)" }}>
                    {sub.last_fetch_at ? formatRelativeTime(sub.last_fetch_at) : "—"}
                  </div>
                  <div className="col-span-2 text-lg tabular-nums" style={{ color: "var(--color-ink-muted)" }}>
                    <div>{formatInterval(sub.fetch_interval_sec)}</div>
                    {formatFetchWindowLine(
                      sub.fetch_window_start_min ?? -1,
                      sub.fetch_window_end_min ?? -1
                    ) && (
                      <div className="text-xs mt-0.5 font-normal" style={{ color: "var(--color-ink-faint)" }}>
                        仅{" "}
                        {formatFetchWindowLine(
                          sub.fetch_window_start_min ?? -1,
                          sub.fetch_window_end_min ?? -1
                        )}
                      </div>
                    )}
                  </div>
                  <div className="col-span-2 flex justify-end items-center gap-2">
                    <button
                      type="button"
                      onClick={() => setPendingDelete(sub)}
                      className="text-sm px-3 py-1.5 rounded-md border-2 font-medium transition-colors hover:bg-red-50/80"
                      style={{ borderColor: "var(--color-danger)", color: "var(--color-danger)" }}
                    >
                      删除
                    </button>
                    <RowActions
                      sub={sub}
                      copied={copiedId === sub.id}
                      onCopy={() => handleCopyFeed(sub)}
                      onSchedule={() => setScheduleFor(sub)}
                    />
                  </div>
                </div>
              ))}
            </div>
            {filtered.length === 0 && searchQuery && (
              <p className="text-center py-10 text-sm" style={{ color: "var(--color-ink-muted)" }}>
                无匹配项
              </p>
            )}
          </>
        )}
      </div>

      {modalOpen && <AddSubscriptionModal onClose={() => setModalOpen(false)} />}

      {scheduleFor && (
        <SubscriptionScheduleModal subscription={scheduleFor} onClose={() => setScheduleFor(null)} />
      )}

      <ConfirmDialog
        open={!!pendingDelete}
        title="删除订阅？"
        description={pendingDelete ? `将移除「${pendingDelete.alias}」，不可恢复。` : ""}
        confirmText="删除"
        cancelText="取消"
        onConfirm={() => {
          if (pendingDelete) deleteMutation.mutate(pendingDelete.id);
          setPendingDelete(null);
        }}
        onCancel={() => setPendingDelete(null)}
      />
    </div>
  );
}
