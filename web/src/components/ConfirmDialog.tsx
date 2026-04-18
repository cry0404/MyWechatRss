import { ModalPortal } from "@/components/ModalPortal";

interface ConfirmDialogProps {
  open: boolean;
  title: string;
  description: string;
  confirmText?: string;
  cancelText?: string;
  onConfirm: () => void;
  onCancel: () => void;
}

export function ConfirmDialog({
  open,
  title,
  description,
  confirmText = "确认",
  cancelText = "取消",
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  if (!open) return null;

  return (
    <ModalPortal>
      <div className="fixed inset-0 z-[1000] flex items-center justify-center p-4">
        <div className="absolute inset-0 z-0 bg-black/40" onClick={onCancel} aria-hidden />
        <div
          className="relative z-[1] w-full max-w-sm border-2 bg-white p-6 rounded-xl"
          style={{ borderColor: "var(--color-border)" }}
        >
        <h3 className="text-2xl font-heading mb-2">{title}</h3>
        <p className="text-sm leading-relaxed mb-6" style={{ color: "var(--color-ink-muted)" }}>
          {description}
        </p>
        <div className="flex gap-2 justify-end">
          <button type="button" onClick={onCancel} className="btn-secondary rounded text-sm px-4 py-2">
            {cancelText}
          </button>
          <button
            type="button"
            onClick={onConfirm}
            className="rounded text-sm px-4 py-2 text-white font-medium"
            style={{ backgroundColor: "var(--color-danger)" }}
          >
            {confirmText}
          </button>
        </div>
        </div>
      </div>
    </ModalPortal>
  );
}
