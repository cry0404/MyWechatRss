import { useState, useMemo } from "react";
import { Link } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Loader2,
  Plus,
  Download,
  MoreHorizontal,
  RefreshCw,
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
          aria-label="更多操作"
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

function AddSubscriptionModal({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: (subscription: Subscription) => void;
}) {
  const showAlert = useAlertStore((s) => s.show);
  const [query, setQuery] = useState("");
  const [inputValue, setInputValue] = useState("");
  const [searchValidation, setSearchValidation] = useState("");
  const [selected, setSelected] = useState<SearchResult | null>(null);
  const [alias, setAlias] = useState("");

  const searchQuery = useQuery({
    queryKey: ["search", query],
    queryFn: () => api.search(query),
    enabled: query.length >= 2,
    staleTime: 60_000,
  });

  const createMutation = useMutation({
    mutationFn: (data: { book_id: string; alias: string }) =>
      api.createSubscription(data.book_id, data.alias),
    onSuccess: (sub) => {
      onCreated(sub);
      onClose();
    },
    onError: (err) => showAlert(toUserMessage(err)),
  });

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault();
    const nextQuery = inputValue.trim();
    if (nextQuery.length < 2) {
      setSearchValidation("请输入至少 2 个字符，或粘贴完整的 MP_WXS_ ID。");
      return;
    }
    setSearchValidation("");
    if (nextQuery === query) {
      searchQuery.refetch();
      return;
    }
    setQuery(nextQuery);
  };

  const handleConfirm = () => {
    if (!selected || !alias.trim()) return;
    createMutation.mutate({ book_id: selected.book_id, alias: alias.trim() });
  };

  return (
    <ModalPortal onClose={onClose}>
      <div className="fixed inset-0 z-[1000] flex items-start justify-center pt-[12vh] px-4">
        <div className="absolute inset-0 z-0 bg-black/45" onClick={onClose} aria-hidden />
        <div
          className="relative z-[1] w-full max-w-lg border-2 bg-white overflow-hidden rounded-xl"
          style={{ borderColor: "var(--color-border)" }}
          onClick={(e) => e.stopPropagation()}
          role="dialog"
          aria-modal="true"
          aria-labelledby="add-subscription-title"
        >
        <div className="flex items-center justify-between px-5 py-4 border-b-2" style={{ borderColor: "var(--color-border-soft)" }}>
          <h3 id="add-subscription-title" className="text-xl font-heading">{selected ? "确认订阅" : "添加订阅"}</h3>
          <button type="button" onClick={onClose} className="text-xs" style={{ color: "var(--color-ink-muted)" }}>
            关闭
          </button>
        </div>

        <div className="px-5 py-5">
          {!selected ? (
            <>
              <form onSubmit={handleSearch} className="flex flex-col sm:flex-row gap-2 mb-3">
                <input
                  type="text"
                  aria-label="搜索公众号"
                  placeholder="搜索公众号名称或粘贴 MP_WXS_ ID"
                  value={inputValue}
                  onChange={(e) => {
                    setInputValue(e.target.value);
                    setSearchValidation("");
                  }}
                  className="input-search-pill text-lg flex-1 min-w-0"
                  autoFocus
                  data-autofocus
                />
                <button
                  type="submit"
                  disabled={searchQuery.isFetching}
                  className="btn-primary shrink-0 disabled:opacity-50"
                >
                  {searchQuery.isFetching ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
                  搜索
                </button>
              </form>

              {searchValidation ? (
                <p className="text-sm mb-3" style={{ color: "var(--color-danger)" }} role="alert">
                  {searchValidation}
                </p>
              ) : null}

              {searchQuery.isFetching && (
                <div className="flex justify-center py-8">
                  <Loader2 className="h-5 w-5 animate-spin" style={{ color: "var(--color-ink-muted)" }} />
                </div>
              )}

              {searchQuery.isError && !searchQuery.isFetching ? (
                <div className="rounded-lg border-2 px-4 py-4" style={{ borderColor: "var(--color-danger)" }} role="alert">
                  <p className="text-sm mb-3" style={{ color: "var(--color-danger)" }}>
                    {toUserMessage(searchQuery.error)}
                  </p>
                  <button type="button" onClick={() => searchQuery.refetch()} className="btn-secondary text-sm px-3 py-1.5">
                    重试
                  </button>
                </div>
              ) : null}

              {!searchQuery.isFetching && !searchQuery.isError && searchQuery.data && searchQuery.data.length === 0 && query.length >= 2 && (
                <p className="text-center py-8 text-sm" style={{ color: "var(--color-ink-muted)" }}>
                  没有找到匹配的公众号，请换个名称或粘贴 MP_WXS_ ID。
                </p>
              )}

              {!searchQuery.isFetching && !searchQuery.isError && searchQuery.data && searchQuery.data.length > 0 && (
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
                <label htmlFor="subscription-alias" className="block text-xs font-medium mb-1.5" style={{ color: "var(--color-ink-muted)" }}>
                  显示名称
                </label>
                <input
                  id="subscription-alias"
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
                      添加中…
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

type InitialFetchState =
  | { status: "pending" }
  | { status: "success"; newCount: number }
  | { status: "error"; message: string };

function FirstFetchStatus({
  state,
  lastFetchAt,
  onRetry,
}: {
  state?: InitialFetchState;
  lastFetchAt?: number;
  onRetry: () => void;
}) {
  if (state?.status === "pending") {
    return (
      <span className="inline-flex items-center gap-1.5 text-sm" style={{ color: "var(--color-secondary)" }}>
        <Loader2 className="h-3.5 w-3.5 animate-spin" />
        首次抓取中…
      </span>
    );
  }
  if (state?.status === "success") {
    return (
      <span className="text-sm" style={{ color: "var(--color-success)" }}>
        首次抓取完成 · 新增 {state.newCount} 篇
      </span>
    );
  }
  if (state?.status === "error") {
    return (
      <div>
        <p className="text-sm mb-1" style={{ color: "var(--color-danger)" }}>
          首次抓取失败
        </p>
        <p className="text-xs line-clamp-2 mb-1" style={{ color: "var(--color-ink-muted)" }}>
          {state.message}
        </p>
        <button type="button" onClick={onRetry} className="text-xs underline underline-offset-2">
          重试
        </button>
      </div>
    );
  }
  return (
    <span className="text-lg" style={{ color: "var(--color-ink-light)" }}>
      {lastFetchAt ? formatRelativeTime(lastFetchAt) : "等待首次抓取"}
    </span>
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
  const [initialFetches, setInitialFetches] = useState<Record<number, InitialFetchState>>({});
  const queryClient = useQueryClient();

  const subscriptionsQuery = useQuery({
    queryKey: ["subscriptions"],
    queryFn: () => api.getSubscriptions(),
  });

  const accountsQuery = useQuery({
    queryKey: ["accounts"],
    queryFn: () => api.getAccounts(),
  });
  const subscriptions = subscriptionsQuery.data;
  const accounts = accountsQuery.data ?? [];
  const isLoading = subscriptionsQuery.isLoading || accountsQuery.isLoading;
  const isLoadError = subscriptionsQuery.isError || accountsQuery.isError;
  const activeAccountCount = accounts.filter((account) => account.status === "active").length;
  const canAddSubscription = activeAccountCount > 0;

  const runInitialFetch = async (subscriptionId: number) => {
    setInitialFetches((current) => ({
      ...current,
      [subscriptionId]: { status: "pending" },
    }));
    try {
      const result = await api.refreshSubscription(subscriptionId);
      setInitialFetches((current) => ({
        ...current,
        [subscriptionId]: { status: "success", newCount: result.new_count },
      }));
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["subscriptions"] }),
        queryClient.invalidateQueries({ queryKey: ["articles", subscriptionId] }),
        queryClient.invalidateQueries({ queryKey: ["global-articles"] }),
      ]);
    } catch (error) {
      setInitialFetches((current) => ({
        ...current,
        [subscriptionId]: { status: "error", message: toUserMessage(error) },
      }));
    }
  };

  const handleSubscriptionCreated = (subscription: Subscription) => {
    queryClient.setQueryData<Subscription[]>(["subscriptions"], (current) => {
      if (!current) return [subscription];
      if (current.some((item) => item.id === subscription.id)) return current;
      return [subscription, ...current];
    });
    void runInitialFetch(subscription.id);
  };

  const deleteMutation = useMutation({
    mutationFn: (id: number) => api.deleteSubscription(id),
    onSuccess: (_data, deletedId) => {
      queryClient.invalidateQueries({ queryKey: ["subscriptions"] });
      setInitialFetches((current) => {
        const next = { ...current };
        delete next[deletedId];
        return next;
      });
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

  const refreshAllMutation = useMutation({
    mutationFn: () => api.refreshAllSubscriptions(),
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ["subscriptions"] });
      queryClient.invalidateQueries({ queryKey: ["articles"] });
      showAlert(`全部拉取完成，新增 ${data.total_new} 篇文章`, "success");
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

  const exportOPML = () => {
    if (!subscriptions?.length) return;

    const escapeXml = (str: string) =>
      str
        .replace(/&/g, "&amp;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;")
        .replace(/"/g, "&quot;");

    let opml = `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head>
    <title>WeChatRead Subscriptions</title>
    <dateCreated>${new Date().toUTCString()}</dateCreated>
  </head>
  <body>
`;

    for (const sub of subscriptions) {
      const xmlUrl = `${window.location.origin}/rss/${sub.feed_id}`;
      opml += `    <outline text="${escapeXml(sub.alias)}" title="${escapeXml(sub.alias)}" type="rss" xmlUrl="${escapeXml(xmlUrl)}" htmlUrl="${escapeXml(xmlUrl)}"/>
`;
    }

    opml += `  </body>
</opml>`;

    const blob = new Blob([opml], { type: "application/xml" });
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = `wechatread-subscriptions-${Date.now()}.opml`;
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
    const wereadOk = activeAccountCount;
    return { total, active, disabled, wereadOk };
  }, [subscriptions, activeAccountCount]);

  return (
    <div className="page-enter w-full">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-8">
        <h1 className="text-4xl md:text-5xl font-heading" style={{ color: "var(--color-ink)" }}>
          订阅
        </h1>
        <div className="flex flex-wrap items-center gap-2">
          {subscriptions && subscriptions.length > 0 ? (
            <>
              <button
                type="button"
                onClick={() => refreshAllMutation.mutate()}
                disabled={!canAddSubscription || refreshAllMutation.isPending}
                className="btn-secondary rounded-full text-base px-4 py-2 disabled:opacity-40"
              >
                {refreshAllMutation.isPending ? (
                  <Loader2 className="h-4 w-4 animate-spin" strokeWidth={2.5} />
                ) : (
                  <RefreshCw className="h-4 w-4 opacity-80" strokeWidth={2.5} />
                )}
                {refreshAllMutation.isPending ? "拉取中…" : "全部拉取"}
              </button>
              <button
                type="button"
                onClick={exportOPML}
                className="btn-secondary rounded-full text-base px-4 py-2"
              >
                <Download className="h-4 w-4 opacity-80" strokeWidth={2.5} />
                导出 OPML
              </button>
            </>
          ) : null}
          {subscriptions && subscriptions.length > 0 ? (
            canAddSubscription ? (
              <button
                type="button"
                onClick={() => setModalOpen(true)}
                className="btn-primary rounded-full text-base px-5 py-2.5 gap-1.5"
              >
                <Plus className="h-4 w-4" strokeWidth={2.5} />
                添加
              </button>
            ) : (
              <Link to="/accounts" className="btn-primary rounded-full text-base px-5 py-2.5">
                {accounts.length > 0 ? "查看账号状态" : "绑定账号"}
              </Link>
            )
          ) : null}
        </div>
      </div>

      {subscriptions && subscriptions.length > 0 ? (
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
          <StatMetric label="订阅总数" value={stats.total} dotColor="var(--color-stat-gray)" />
          <StatMetric label="已启用" value={stats.active} dotColor="var(--color-stat-purple)" />
          <StatMetric label="已停用" value={stats.disabled} dotColor="var(--color-stat-orange)" />
          <StatMetric label="可用读书账号" value={stats.wereadOk} dotColor="var(--color-stat-green)" />
        </div>
      ) : null}

      <div className="panel-elevated">
        {subscriptions && subscriptions.length > 0 ? (
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
        ) : null}

        {isLoading ? (
          <div className="flex justify-center py-20">
            <Loader2 className="h-5 w-5 animate-spin" style={{ color: "var(--color-ink-muted)" }} />
          </div>
        ) : isLoadError ? (
          <div className="py-16 px-5 text-center" role="alert">
            <p className="text-xl font-heading mb-2">订阅状态加载失败</p>
            <p className="text-sm mb-5" style={{ color: "var(--color-ink-muted)" }}>
              请检查服务连接后重试。
            </p>
            <button
              type="button"
              onClick={() => {
                subscriptionsQuery.refetch();
                accountsQuery.refetch();
              }}
              className="btn-primary rounded-full text-base px-5 py-2.5"
            >
              重新加载
            </button>
          </div>
        ) : !subscriptions || subscriptions.length === 0 ? (
          <div className="py-16 px-5 text-center">
            <p className="text-xl font-heading mb-1">还没有订阅</p>
            <p className="text-lg mb-5" style={{ color: "var(--color-ink-muted)" }}>
              {canAddSubscription
                ? "添加公众号后会自动进行首次抓取"
                : accounts.length > 0
                  ? "当前没有可用账号，请先恢复账号状态"
                  : "添加订阅前，需要先绑定微信读书账号"}
            </p>
            {canAddSubscription ? (
              <button type="button" onClick={() => setModalOpen(true)} className="btn-primary rounded-full text-base px-5 py-2.5 gap-1.5">
                <Plus className="h-4 w-4" strokeWidth={2.5} />
                添加
              </button>
            ) : (
              <Link to="/accounts" className="btn-primary rounded-full text-base px-5 py-2.5">
                {accounts.length > 0 ? "查看账号状态" : "去绑定账号"}
              </Link>
            )}
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
                  <div className="col-span-2">
                    <span className="sm:hidden text-xs mr-2" style={{ color: "var(--color-ink-faint)" }}>
                      抓取状态
                    </span>
                    <FirstFetchStatus
                      state={initialFetches[sub.id]}
                      lastFetchAt={sub.last_fetch_at}
                      onRetry={() => void runInitialFetch(sub.id)}
                    />
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

      {modalOpen && canAddSubscription && (
        <AddSubscriptionModal
          onClose={() => setModalOpen(false)}
          onCreated={handleSubscriptionCreated}
        />
      )}

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
