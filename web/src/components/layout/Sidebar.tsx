import { Link, useLocation } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import {
  LayoutDashboard,
  BookOpen,
  Rss,
  Users,
  Settings,
  LogOut,
} from "lucide-react";
import { cn } from "@/lib/cn";
import { useAuthStore } from "@/stores/authStore";
import { useLogout } from "@/hooks/useAuth";
import { api } from "@/lib/api";

const mainNavItems = [
  { path: "/", label: "概览", icon: LayoutDashboard },
  { path: "/subscriptions", label: "订阅", icon: BookOpen },
  { path: "/feeds", label: "文章流", icon: Rss },
];

const manageNavItems = [
  { path: "/accounts", label: "账号", icon: Users },
  { path: "/settings", label: "设置", icon: Settings },
];

export function Sidebar() {
  const location = useLocation();
  const logout = useLogout();
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);

  const { data: subscriptions } = useQuery({
    queryKey: ["subscriptions"],
    queryFn: () => api.getSubscriptions(),
    staleTime: 30_000,
  });
  const subCount = subscriptions?.length ?? 0;

  const isActive = (path: string) =>
    location.pathname === path || location.pathname.startsWith(path + "/");

  return (
    <aside
      className="fixed left-0 top-0 z-10 flex h-screen w-56 flex-col border-r-2"
      style={{ backgroundColor: "var(--color-bg-sidebar)", borderColor: "var(--color-border)" }}
    >
      <div className="px-4 py-5 flex items-center gap-3 border-b-2" style={{ borderColor: "var(--color-border)" }}>
        <span className="font-heading text-lg leading-tight truncate" style={{ color: "var(--color-ink)" }}>
          WeChatRead RSS
        </span>
      </div>

      <nav className="flex-1 px-3 py-4 space-y-6 overflow-y-auto">
        <div className="space-y-0.5">
          {mainNavItems.map((item) => {
            const active = isActive(item.path);
            const Icon = item.icon;
            return (
              <Link key={item.path} to={item.path} className={cn("sidebar-link", active && "active")}>
                <Icon className="h-4 w-4 shrink-0 opacity-90" strokeWidth={2.25} />
                <span className="flex-1">{item.label}</span>
                {item.path === "/subscriptions" && subCount > 0 && (
                  <span
                    className="text-[10px] font-semibold tabular-nums min-w-[1.25rem] text-center px-1.5 py-0.5 rounded-md"
                    style={{ backgroundColor: "var(--color-bg-hover)", color: "var(--color-ink-muted)" }}
                  >
                    {subCount}
                  </span>
                )}
              </Link>
            );
          })}
        </div>

        <div>
          <p
            className="px-3 mb-2 text-[10px] font-semibold uppercase tracking-widest"
            style={{ color: "var(--color-ink-faint)" }}
          >
            管理
          </p>
          <div className="space-y-0.5">
            {manageNavItems.map((item) => {
              const active = isActive(item.path);
              const Icon = item.icon;
              return (
                <Link key={item.path} to={item.path} className={cn("sidebar-link", active && "active")}>
                  <Icon className="h-4 w-4 shrink-0 opacity-90" strokeWidth={2.25} />
                  <span>{item.label}</span>
                </Link>
              );
            })}
          </div>
        </div>
      </nav>

      {isAuthenticated && (
        <div className="px-3 py-3 border-t" style={{ borderColor: "var(--color-border)" }}>
          <button
            type="button"
            onClick={logout}
            className="sidebar-link w-full border-0 !justify-start"
            style={{ color: "var(--color-ink-muted)" }}
            onMouseEnter={(e) => {
              e.currentTarget.style.color = "var(--color-danger)";
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.color = "var(--color-ink-muted)";
            }}
          >
            <LogOut className="h-4 w-4 shrink-0 opacity-90" strokeWidth={2.25} />
            <span>退出登录</span>
          </button>
        </div>
      )}
    </aside>
  );
}
