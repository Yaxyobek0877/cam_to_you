/**
 * Go backend (models paketi) bilan mos keluvchi TypeScript interfeyslari.
 *
 * Wails `wails generate module` orqali ham TS tiplari yaratadi (frontend/wailsjs/go/),
 * lekin bularni qo'lda yozish — yaqinroq nazorat va IDE awareness uchun.
 */

export type CameraVendor = "hikvision" | "dahua" | "generic";

export interface Camera {
  id: number;
  name: string;
  vendor: CameraVendor;
  host: string;
  port: number;
  username: string;
  password: string;
  channel: number;
  useSubStream: boolean;
  rawRtspUrl: string;
  createdAt: string;
  updatedAt: string;
  isOnline?: boolean;
  lastProbed?: string;
}

export type Layout = "single" | "1x2" | "2x1" | "2x2" | "3x3" | "pip";
export type Quality = "720p30" | "720p60" | "1080p30" | "1080p60" | "1440p30";
export type Encoder = "auto" | "h264_nvenc" | "h264_qsv" | "h264_amf" | "libx264" | "libopenh264" | "copy";
export type Platform = "youtube" | "twitch" | "facebook" | "custom";
export type AudioMode = "first" | "muted" | "index";

export interface Stream {
  id: number;
  name: string;
  layout: Layout;
  cameraIds: number[];
  quality: Quality;
  encoder: Encoder;
  audioMode: AudioMode;
  audioCameraIndex: number;
  platform: Platform;
  streamKey: string;
  customUrl: string;
  autoRestart: boolean;
  maxRestarts: number;
  restartDelayMs: number;
  createdAt: string;
  updatedAt: string;
}

export type StreamState =
  | "idle"
  | "starting"
  | "running"
  | "stopping"
  | "stopped"
  | "error";

export interface StreamStatus {
  streamId: number;
  state: StreamState;
  uptime: number;
  lastError: string;
  startedAt?: string;
  restartCount: number;
}

export interface ProbeResult {
  ok: boolean;
  codec: string;
  width: number;
  height: number;
  fps: string;
  hasAudio: boolean;
  audioCodec: string;
  error?: string;
}

export interface FFmpegStatus {
  installed: boolean;
  path: string;
  version: string;
}

export interface HardwareInfo {
  hasGpu: boolean;
  hasNvenc: boolean;
  hasQuickSync: boolean;
  hasAmf: boolean;
  hasX264: boolean;
  hasOpenH264: boolean;
  hasMediaFound: boolean;
  bestEncoder: string;
  recommendation: string;
}

export interface FFmpegProgress {
  stage: "downloading" | "extracting" | "done";
  percent: number;
  speedMBps: number; // yuklash tezligi
  etaSec: number;    // qolgan vaqt sekundlarda (0 = noma'lum)
  message: string;
}

export interface StreamEvent {
  type: "state_change" | "log" | "error" | "restart";
  streamId: number;
  payload?: unknown;
}
