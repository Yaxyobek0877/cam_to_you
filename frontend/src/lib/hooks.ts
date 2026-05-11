import { useEffect } from "react";

/**
 * useEscapeKey — Escape tugmasi bosilganda berilgan funksiyani chaqiradi.
 * Modal'lar uchun: foydalanuvchi Esc bossa, modal yopilsin.
 */
export function useEscapeKey(handler: () => void) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") handler();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [handler]);
}
