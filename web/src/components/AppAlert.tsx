import { useEffect } from "react";
import { CheckCircle2, CircleAlert, Info, X } from "lucide-react";
import { useAlertStore } from "@/stores/alertStore";
import { ModalPortal } from "@/components/ModalPortal";

export function AppAlert() {
  const message = useAlertStore((s) => s.message);
  const tone = useAlertStore((s) => s.tone);
  const hide = useAlertStore((s) => s.hide);

  useEffect(() => {
    if (!message) return;
    const timer = window.setTimeout(hide, tone === "error" ? 6000 : 3500);
    return () => window.clearTimeout(timer);
  }, [hide, message, tone]);

  if (!message) return null;

  return (
    <ModalPortal lockScroll={false}>
      <div className="pointer-events-none fixed inset-x-0 top-4 z-[1100] flex justify-center px-4 sm:top-6">
        <div
          className="pointer-events-auto flex w-full max-w-md items-start gap-3 rounded-xl border-2 bg-white p-4"
          style={{
            borderColor:
              tone === "error"
                ? "var(--color-danger)"
                : tone === "success"
                  ? "var(--color-success)"
                  : "var(--color-secondary)",
          }}
          role={tone === "error" ? "alert" : "status"}
          aria-live={tone === "error" ? "assertive" : "polite"}
        >
          {tone === "error" ? (
            <CircleAlert className="mt-0.5 h-5 w-5 shrink-0" style={{ color: "var(--color-danger)" }} />
          ) : tone === "success" ? (
            <CheckCircle2 className="mt-0.5 h-5 w-5 shrink-0" style={{ color: "var(--color-success)" }} />
          ) : (
            <Info className="mt-0.5 h-5 w-5 shrink-0" style={{ color: "var(--color-secondary)" }} />
          )}
          <p className="flex-1 text-sm leading-relaxed whitespace-pre-wrap" style={{ color: "var(--color-ink)" }}>
            {message}
          </p>
          <button type="button" onClick={hide} className="-mr-1 -mt-1 rounded p-1" aria-label="关闭提示">
            <X className="h-4 w-4" />
          </button>
        </div>
      </div>
    </ModalPortal>
  );
}
