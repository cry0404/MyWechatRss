import { useAlertStore } from "@/stores/alertStore";
import { ModalPortal } from "@/components/ModalPortal";

export function AppAlert() {
  const message = useAlertStore((s) => s.message);
  const hide = useAlertStore((s) => s.hide);

  if (!message) return null;

  return (
    <ModalPortal>
      <div className="fixed inset-0 z-[1000] flex items-center justify-center p-4">
        <div className="absolute inset-0 z-0 bg-black/40" onClick={hide} aria-hidden />
        <div
          className="relative z-[1] w-full max-w-sm border-2 rounded-2xl bg-white p-6"
          style={{ borderColor: "var(--color-border)" }}
          role="alertdialog"
          aria-labelledby="app-alert-title"
          aria-describedby="app-alert-desc"
        >
          <h3 id="app-alert-title" className="text-xl font-heading mb-3" style={{ color: "var(--color-ink)" }}>
            Notice
          </h3>
          <p id="app-alert-desc" className="text-base leading-relaxed mb-6 whitespace-pre-wrap" style={{ color: "var(--color-ink-muted)" }}>
            {message}
          </p>
          <div className="flex justify-end">
            <button type="button" onClick={hide} className="btn-primary !rounded-xl px-5 py-2.5">
              OK
            </button>
          </div>
        </div>
      </div>
    </ModalPortal>
  );
}
