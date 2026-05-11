/**
 * Global event store — barcha Go'dan kelgan hodisalarni vaqtli tartibda saqlaydi.
 * Logs sahifasi shu store'dan o'qiydi.
 *
 * Foydalanish:
 *   1. App boot'ida initEventStore() chaqiriladi
 *   2. Hodisalar avtomatik ringbuffer'ga yoziladi (oxirgi 1000)
 *   3. UI komponentlari useEventLog() bilan reaktiv tinglashlari mumkin
 */

import { onStreamEvent, onPreviewEvent, onFFmpegProgress } from "./api";

export type LogSource = "stream" | "preview" | "ffmpeg" | "system";
export type LogLevel = "info" | "warning" | "error";

export interface LogEntry {
  id: number;
  time: Date;
  source: LogSource;
  sourceId?: number;     // streamId yoki cameraId
  level: LogLevel;
  type: string;          // event turi: state_change, log, error, ready, ...
  message: string;
  raw?: any;             // asl payload (debug uchun)
}

const MAX_ENTRIES = 1000;
let entries: LogEntry[] = [];
let nextId = 1;
let listeners: Array<(entries: LogEntry[]) => void> = [];

function add(entry: Omit<LogEntry, "id">) {
  const full: LogEntry = { ...entry, id: nextId++ };
  entries = [...entries, full];
  if (entries.length > MAX_ENTRIES) {
    entries = entries.slice(entries.length - MAX_ENTRIES);
  }
  listeners.forEach((l) => l(entries));
}

export function getEntries(): LogEntry[] {
  return entries;
}

export function clearEntries() {
  entries = [];
  listeners.forEach((l) => l(entries));
}

export function subscribe(listener: (entries: LogEntry[]) => void): () => void {
  listeners.push(listener);
  return () => {
    listeners = listeners.filter((l) => l !== listener);
  };
}

let initialized = false;
export function initEventStore() {
  if (initialized) return;
  initialized = true;

  // Boshlanish hodisasi
  add({
    time: new Date(),
    source: "system",
    level: "info",
    type: "boot",
    message: "Cam2You ishga tushdi",
  });

  // Stream hodisalari
  onStreamEvent((data: any) => {
    const ev = data as { type: string; streamId: number; payload?: any };
    let level: LogLevel = "info";
    let message = "";

    switch (ev.type) {
      case "state_change":
        message = `Holat: ${ev.payload}`;
        if (ev.payload === "error") level = "error";
        if (ev.payload === "running") level = "info";
        break;
      case "log":
        level = (ev.payload?.level as LogLevel) || "info";
        message = ev.payload?.message || "(bo'sh log)";
        break;
      case "error":
        level = "error";
        message = typeof ev.payload === "string" ? ev.payload : JSON.stringify(ev.payload);
        break;
      case "restart":
        level = "warning";
        message = `Qayta urinish #${ev.payload}`;
        break;
      default:
        message = ev.type;
    }

    add({
      time: new Date(),
      source: "stream",
      sourceId: ev.streamId,
      level,
      type: ev.type,
      message,
      raw: ev,
    });
  });

  // Preview hodisalari
  onPreviewEvent((data: any) => {
    const ev = data as { type: string; cameraId: number; payload?: any };
    let level: LogLevel = "info";
    let message = "";

    switch (ev.type) {
      case "starting":
        message = `Preview boshlandi (encoder: ${ev.payload?.encoder || "?"})`;
        break;
      case "ready":
        message = "Video oqimi tayyor";
        break;
      case "log":
        level = (ev.payload?.level as LogLevel) || "info";
        message = ev.payload?.message || "(bo'sh log)";
        break;
      case "error":
        level = "error";
        message = typeof ev.payload === "string" ? ev.payload : JSON.stringify(ev.payload);
        break;
      case "stopped":
        message = "Preview to'xtatildi";
        break;
      default:
        message = ev.type;
    }

    add({
      time: new Date(),
      source: "preview",
      sourceId: ev.cameraId,
      level,
      type: ev.type,
      message,
      raw: ev,
    });
  });

  // FFmpeg progress (auto-installer)
  onFFmpegProgress((data: any) => {
    const ev = data as { stage: string; percent: number; speedMBps: number; message: string };
    if (ev.stage === "done") {
      add({
        time: new Date(),
        source: "ffmpeg",
        level: "info",
        type: "install_done",
        message: ev.message,
        raw: ev,
      });
    }
  });
}
