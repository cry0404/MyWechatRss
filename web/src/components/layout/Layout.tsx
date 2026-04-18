import { Outlet } from "react-router-dom";
import { Sidebar } from "./Sidebar";
import { AccountHealthBanner } from "./AccountHealthBanner";

export function Layout() {
  return (
    <div className="min-h-screen" style={{ backgroundColor: "var(--color-bg)" }}>
      <Sidebar />
      <main
        className="relative z-20 min-h-screen bg-transparent"
        style={{ marginLeft: "14rem" }}
      >
        <div className="page-enter p-6 sm:p-8 max-w-7xl mx-auto">
          <AccountHealthBanner />
          <Outlet />
        </div>
      </main>
    </div>
  );
}
