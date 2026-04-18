import { useEffect, useState } from "react";
import { Loader2 } from "lucide-react";
import { api } from "@/lib/api";

export function AuthInit({ children }: { children: React.ReactNode }) {
  const [checking, setChecking] = useState(true);

  useEffect(() => {
    const token = localStorage.getItem("token");
    if (!token) {
      setChecking(false);
      return;
    }
    api
      .getMe()
      .then(() => setChecking(false))
      .catch(() => setChecking(false));
  }, []);

  if (checking) {
    return (
      <div
        className="flex items-center justify-center h-screen"
        style={{ backgroundColor: "var(--color-bg)" }}
      >
        <Loader2 className="w-8 h-8 animate-spin" style={{ color: "var(--color-ink-muted)" }} />
      </div>
    );
  }

  return <>{children}</>;
}
