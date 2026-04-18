import { useEffect } from "react";
import { createPortal } from "react-dom";

export function ModalPortal({
  children,
  lockScroll = true,
}: {
  children: React.ReactNode;
  lockScroll?: boolean;
}) {
  useEffect(() => {
    if (!lockScroll) return;
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = prev;
    };
  }, [lockScroll]);

  return createPortal(children, document.body);
}
