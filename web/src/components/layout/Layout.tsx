import { useState } from "react";
import { Link, Outlet, useLocation } from "react-router-dom";
import { ShieldAlert, Menu } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { Sidebar } from "./Sidebar";
import { AccountHealthBanner } from "./AccountHealthBanner";
import { api } from "@/lib/api";

function currentPageLabel(pathname: string) {
  if (pathname === "/") return "首页";
  if (pathname.startsWith("/subscriptions/")) return "订阅详情";
  if (pathname.startsWith("/subscriptions")) return "订阅";
  if (pathname.startsWith("/accounts")) return "账号";
  if (pathname.startsWith("/feeds")) return "文章流";
  if (pathname.startsWith("/logs")) return "抓取记录";
  if (pathname.startsWith("/settings")) return "设置";
  return "WeChatRead RSS";
}

export function Layout() {
  const [mobileOpen, setMobileOpen] = useState(false);
  const location = useLocation();
  const { data: me } = useQuery({ queryKey: ["me"], queryFn: api.getMe });
  const pageLabel = currentPageLabel(location.pathname);

  return (
    <div className="min-h-screen" style={{ backgroundColor: "var(--color-bg)" }}>
      {/* Mobile overlay */}
      {mobileOpen && (
        <div
          className="fixed inset-0 z-30 bg-black/30 md:hidden"
          onClick={() => setMobileOpen(false)}
        />
      )}

      <Sidebar mobileOpen={mobileOpen} onClose={() => setMobileOpen(false)} />

      {/* Mobile header bar */}
      <header className="md:hidden sticky top-0 z-20 flex items-center gap-3 px-4 py-3 border-b-2"
        style={{ backgroundColor: "var(--color-bg)", borderColor: "var(--color-border)" }}
      >
        <button
          type="button"
          onClick={() => setMobileOpen(true)}
          className="p-1.5 -ml-1 rounded border-2"
          style={{ borderColor: "var(--color-border)" }}
          aria-label="打开菜单"
        >
          <Menu className="h-5 w-5" />
        </button>
        <span className="font-heading text-base leading-tight truncate" style={{ color: "var(--color-ink)" }}>
          {pageLabel}
        </span>
      </header>

      <main className="relative z-10 min-h-screen bg-transparent md:ml-56">
        <div className="page-enter p-4 sm:p-6 md:p-8 max-w-7xl mx-auto">
          {me?.must_change_password && (
            <div
              className="mb-4 flex flex-col gap-3 rounded-xl border-2 p-4 sm:flex-row sm:items-center sm:justify-between"
              style={{ borderColor: "var(--color-warn)", backgroundColor: "var(--color-warn-pale)" }}
              role="alert"
            >
              <div className="flex items-start gap-3">
                <ShieldAlert className="mt-0.5 h-5 w-5 shrink-0" style={{ color: "var(--color-warn)" }} />
                <div>
                  <p className="text-sm font-medium">当前仍在使用初始默认密码</p>
                  <p className="mt-0.5 text-xs" style={{ color: "var(--color-ink-muted)" }}>
                    为避免他人登录，请先修改密码再继续使用。
                  </p>
                </div>
              </div>
              <Link to="/settings#password" className="btn-primary shrink-0 px-4 py-2 text-sm">
                立即修改
              </Link>
            </div>
          )}
          <AccountHealthBanner />
          <Outlet />
        </div>
      </main>
    </div>
  );
}
