import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

/**
 * Tailwind class'larni birlashtiruvchi yordamchi.
 * cn("p-2", "p-4") → "p-4" (tailwind-merge dublicate'larni yechadi)
 */
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

/** Stream uptime'ni "1h 23m 45s" formatiga aylantiradi. */
export function formatUptime(seconds: number): string {
  if (seconds <= 0) return "—";
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  if (h > 0) return `${h}h ${m}m ${s}s`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
}

/** Kichik vaqt formatlash — "11:23:45" */
export function formatTime(date: Date | string): string {
  const d = typeof date === "string" ? new Date(date) : date;
  return d.toLocaleTimeString("uz-UZ", { hour12: false });
}
