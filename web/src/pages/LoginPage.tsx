import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Loader2, Rss } from "lucide-react";
import { cn } from "@/lib/cn";
import { useLogin } from "@/hooks/useAuth";
import { useAuthStore } from "@/stores/authStore";
import { useAlertStore } from "@/stores/alertStore";
import { toUserMessage } from "@/lib/userMessage";

export default function LoginPage() {
  const navigate = useNavigate();
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const showAlert = useAlertStore((s) => s.show);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");

  const loginMutation = useLogin();

  useEffect(() => {
    if (isAuthenticated) {
      navigate("/", { replace: true });
    }
  }, [isAuthenticated, navigate]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!username.trim() || !password.trim()) {
      showAlert("Please enter username and password.");
      return;
    }
    loginMutation.mutate(
      { username: username.trim(), password },
      {
        onError: (err) => {
          showAlert(toUserMessage(err));
        },
        onSuccess: () => {
          navigate("/", { replace: true });
        },
      }
    );
  };

  const isPending = loginMutation.isPending;

  return (
    <div
      className="relative flex min-h-screen w-full items-center justify-center overflow-hidden px-4 py-12"
      style={{ backgroundColor: "var(--color-bg)" }}
    >
      <div
        className="pointer-events-none absolute inset-0 opacity-[0.45]"
        style={{
          backgroundImage: `
            radial-gradient(ellipse 80% 50% at 10% -10%, color-mix(in srgb, var(--color-secondary) 22%, transparent), transparent 55%),
            radial-gradient(ellipse 70% 45% at 95% 15%, color-mix(in srgb, var(--color-accent) 12%, transparent), transparent 50%),
            radial-gradient(ellipse 60% 40% at 50% 100%, color-mix(in srgb, var(--color-warn) 10%, transparent), transparent 45%)
          `,
        }}
      />

      <div className="relative z-[1] w-full max-w-md">
        <div
          className="overflow-hidden rounded-2xl border-2"
          style={{
            borderColor: "var(--color-border)",
            backgroundColor: "var(--color-bg-surface)",
          }}
        >
          <div className="px-8 pb-8 pt-7">
            <div className="mb-8 flex flex-col items-center text-center">
              <div
                className="mb-4 flex h-14 w-14 items-center justify-center rounded-2xl border-2"
                style={{
                  borderColor: "var(--color-border)",
                  backgroundColor: "var(--color-secondary-pale)",
                }}
              >
                <Rss className="h-7 w-7" strokeWidth={2.2} style={{ color: "var(--color-secondary)" }} />
              </div>
              <h1 className="text-2xl font-heading tracking-tight" style={{ color: "var(--color-ink)" }}>
                WeChatRead RSS
              </h1>
              <p className="mt-1.5 text-base" style={{ color: "var(--color-ink-muted)" }}>
                Sign in to your account
              </p>
            </div>

            <form onSubmit={handleSubmit} className="space-y-5">
              <div>
                <label className="mb-1.5 block text-sm font-medium" style={{ color: "var(--color-ink-muted)" }}>
                  Username
                </label>
                <input
                  type="text"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  className="input-editorial !rounded-xl"
                  autoComplete="username"
                  disabled={isPending}
                />
              </div>

              <div>
                <label className="mb-1.5 block text-sm font-medium" style={{ color: "var(--color-ink-muted)" }}>
                  Password
                </label>
                <input
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  className="input-editorial !rounded-xl"
                  autoComplete="current-password"
                  disabled={isPending}
                />
              </div>

              <button
                type="submit"
                disabled={isPending}
                className={cn(
                  "btn-primary w-full !rounded-xl justify-center min-h-12",
                  isPending && "cursor-not-allowed opacity-70"
                )}
              >
                {isPending ? <Loader2 className="h-5 w-5 animate-spin" /> : "Sign in"}
              </button>
            </form>
          </div>
        </div>

        <p className="mt-6 text-center text-sm" style={{ color: "var(--color-ink-faint)" }}>
          Private RSS feeds for your WeChat Reading subscriptions
        </p>
      </div>
    </div>
  );
}
