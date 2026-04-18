import { useCallback, useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";

const GAP = 6;
const VIEW_MARGIN = 8;
const DEFAULT_MIN_W = 160;

function computeCoords(
  anchor: HTMLElement,
  menu: HTMLElement | null,
  align: "start" | "end"
): { top: number; left: number } {
  const rect = anchor.getBoundingClientRect();
  const mw = Math.max(menu?.offsetWidth ?? DEFAULT_MIN_W, DEFAULT_MIN_W);
  const mh = menu?.offsetHeight ?? 0;

  let left = align === "end" ? rect.right - mw : rect.left;
  left = Math.max(VIEW_MARGIN, Math.min(left, window.innerWidth - mw - VIEW_MARGIN));

  let top = rect.bottom + GAP;
  if (mh > 0 && top + mh > window.innerHeight - VIEW_MARGIN) {
    const above = rect.top - GAP - mh;
    if (above >= VIEW_MARGIN) {
      top = above;
    }
  }
  const clampedH = mh > 0 ? mh : 1;
  top = Math.max(VIEW_MARGIN, Math.min(top, window.innerHeight - clampedH - VIEW_MARGIN));

  return { top, left };
}

export function ActionMenu({
  open,
  onOpenChange,
  align = "end",
  trigger,
  children,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  align?: "start" | "end";
  trigger: ReactNode;
  children: ReactNode;
}) {
  const anchorRef = useRef<HTMLDivElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const [coords, setCoords] = useState<{ top: number; left: number } | null>(null);

  const syncPosition = useCallback(() => {
    const anchor = anchorRef.current;
    if (!anchor) return;
    setCoords(computeCoords(anchor, menuRef.current, align));
  }, [align]);

  useLayoutEffect(() => {
    if (!open) {
      setCoords(null);
      return;
    }
    syncPosition();
    const id = requestAnimationFrame(() => syncPosition());
    return () => cancelAnimationFrame(id);
  }, [open, syncPosition, children]);

  useEffect(() => {
    if (!open) return;
    const onScrollOrResize = () => syncPosition();
    window.addEventListener("scroll", onScrollOrResize, true);
    window.addEventListener("resize", onScrollOrResize);
    return () => {
      window.removeEventListener("scroll", onScrollOrResize, true);
      window.removeEventListener("resize", onScrollOrResize);
    };
  }, [open, syncPosition]);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      const t = e.target as Node;
      if (anchorRef.current?.contains(t)) return;
      if (menuRef.current?.contains(t)) return;
      onOpenChange(false);
    };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [open, onOpenChange]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onOpenChange(false);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [open, onOpenChange]);

  return (
    <>
      <div ref={anchorRef} className="inline-flex items-center justify-center">
        {trigger}
      </div>
      {open &&
        coords &&
        createPortal(
          <div
            ref={menuRef}
            role="menu"
            className="fixed z-[200] min-w-[10rem] overflow-hidden border-2 bg-white py-1.5 text-left rounded-xl"
            style={{
              borderColor: "var(--color-border)",
              top: coords.top,
              left: coords.left,
            }}
          >
            {children}
          </div>,
          document.body
        )}
    </>
  );
}
