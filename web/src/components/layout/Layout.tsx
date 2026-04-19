import { useState } from "react";
import { Outlet } from "react-router-dom";
import { Menu } from "lucide-react";
import { Sidebar } from "./Sidebar";
import { AccountHealthBanner } from "./AccountHealthBanner";

export function Layout() {
  const [mobileOpen, setMobileOpen] = useState(false);

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
          WeChatRead RSS
        </span>
      </header>

      <main className="relative z-10 min-h-screen bg-transparent md:ml-56">
        <div className="page-enter p-4 sm:p-6 md:p-8 max-w-7xl mx-auto">
          <AccountHealthBanner />
          <Outlet />
        </div>
      </main>
    </div>
  );
}
