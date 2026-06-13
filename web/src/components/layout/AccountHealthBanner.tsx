import { useQuery } from "@tanstack/react-query";
import { Link, useLocation } from "react-router-dom";
import { api } from "@/lib/api";

export function AccountHealthBanner() {
  const location = useLocation();
  const { data } = useQuery({
    queryKey: ["accounts"],
    queryFn: () => api.getAccounts(),
    refetchInterval: 30_000,
    staleTime: 10_000,
  });

  if (!data || data.length === 0) return null;
  const hasActive = data.some((a) => a.status === "active");
  if (hasActive) return null;
  if (location.pathname.startsWith("/accounts")) return null;
  const hasDead = data.some((a) => a.status === "dead");
  const title = hasDead ? "所有微信读书账号已失效，抓取已暂停。" : "微信读书账号正在冷却，抓取会自动恢复。";
  const action = hasDead ? "去重新扫码" : "查看账号状态";

  return (
    <div
      className="mb-6 px-4 py-3 text-lg border-2 rounded-md"
      style={{
        backgroundColor: "var(--color-danger-pale)",
        borderColor: "var(--color-danger)",
        color: "var(--color-ink-light)",
      }}
      role="alert"
    >
      <span className="font-medium" style={{ color: "var(--color-danger)" }}>
        {title}
      </span>{" "}
      <Link to="/accounts" className="underline underline-offset-2" style={{ color: "var(--color-ink)" }}>
        {action}
      </Link>
    </div>
  );
}
