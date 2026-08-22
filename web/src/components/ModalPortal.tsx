import { useEffect, useRef } from "react";
import { createPortal } from "react-dom";

export function ModalPortal({
  children,
  lockScroll = true,
  onClose,
}: {
  children: React.ReactNode;
  lockScroll?: boolean;
  onClose?: () => void;
}) {
  const rootRef = useRef<HTMLDivElement>(null);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;
  const managesDialogFocus = Boolean(onClose);

  useEffect(() => {
    if (!lockScroll) return;
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = prev;
    };
  }, [lockScroll]);

  useEffect(() => {
    if (!managesDialogFocus) return;
    const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const root = rootRef.current;
    const focusableSelector =
      'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';
    const first = root?.querySelector<HTMLElement>(`[data-autofocus], ${focusableSelector}`);
    first?.focus();

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onCloseRef.current?.();
        return;
      }
      if (event.key !== "Tab" || !root) return;
      const focusable = Array.from(root.querySelectorAll<HTMLElement>(focusableSelector)).filter(
        (element) => element.offsetParent !== null
      );
      if (focusable.length === 0) {
        event.preventDefault();
        return;
      }
      const firstElement = focusable[0];
      const lastElement = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === firstElement) {
        event.preventDefault();
        lastElement.focus();
      } else if (!event.shiftKey && document.activeElement === lastElement) {
        event.preventDefault();
        firstElement.focus();
      }
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      previousFocus?.focus();
    };
  }, [managesDialogFocus]);

  return createPortal(<div ref={rootRef}>{children}</div>, document.body);
}
