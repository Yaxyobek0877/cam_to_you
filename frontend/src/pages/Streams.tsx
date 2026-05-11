import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import {
  Plus, Radio, Edit2, Trash2, X, Loader2, Play, Square, ScrollText,
} from "lucide-react";
import {
  listStreams, createStream, updateStream, deleteStream,
  startStream, stopStream, listCameras, getAllStreamStatus, getHardwareInfo,
} from "../lib/api";
import { StateBadge } from "./Dashboard";
import { formatUptime } from "../lib/utils";
import { PasswordInput } from "../components/PasswordInput";
import { useEscapeKey } from "../lib/hooks";
import type { Stream, Layout as LayoutType, Quality, Platform, Encoder, AudioMode } from "../lib/types";

const layoutInfo: Record<LayoutType, { label: string; needs: number; preview: string }> = {
  single:  { label: "Bitta kamera",       needs: 1, preview: "□" },
  "1x2":   { label: "2 ta yonma-yon",     needs: 2, preview: "□□" },
  "2x1":   { label: "2 ta ustma-ust",     needs: 2, preview: "▤" },
  "2x2":   { label: "2×2 (4 kamera)",     needs: 4, preview: "▦" },
  "3x3":   { label: "3×3 (9 kamera)",     needs: 9, preview: "▩" },
  pip:     { label: "Picture-in-Picture", needs: 2, preview: "▣" },
};

const emptyStream: Partial<Stream> = {
  name: "",
  layout: "single",
  cameraIds: [],
  quality: "1080p30",
  encoder: "auto",
  audioMode: "first",
  audioCameraIndex: 0,
  platform: "youtube",
  streamKey: "",
  customUrl: "",
  autoRestart: true,
  maxRestarts: 0,
  restartDelayMs: 5000,
};

export function Streams() {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const streamsQ = useQuery({ queryKey: ["streams"], queryFn: listStreams });
  const statusQ = useQuery({
    queryKey: ["allStatus"],
    queryFn: getAllStreamStatus,
    refetchInterval: 2_000,
  });
  const [editing, setEditing] = useState<Partial<Stream> | null>(null);
  const [expandedError, setExpandedError] = useState<number | null>(null);

  const [startError, setStartError] = useState<{ streamId: number; msg: string } | null>(null);
  const startMut = useMutation({
    mutationFn: startStream,
    onSuccess: () => {
      setStartError(null);
      qc.invalidateQueries({ queryKey: ["allStatus"] });
    },
    onError: (err, streamId) => {
      setStartError({ streamId, msg: String(err) });
    },
  });
  const stopMut = useMutation({
    mutationFn: stopStream,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["allStatus"] }),
  });
  const delMut = useMutation({
    mutationFn: deleteStream,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["streams"] }),
  });

  const streams = streamsQ.data ?? [];
  const status = statusQ.data ?? {};

  return (
    <div className="p-6 space-y-6">
      <header className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Streamlar</h1>
          <p className="text-sm text-gray-400 mt-1">
            YouTube va boshqa platformalarga uzatuvchi konfiguratsiyalar
          </p>
        </div>
        <button onClick={() => setEditing(emptyStream)} className="btn-primary">
          <Plus className="w-4 h-4" />
          Yangi stream
        </button>
      </header>

      {streamsQ.isLoading ? (
        <div className="card p-12 flex items-center justify-center text-gray-500">
          <Loader2 className="w-5 h-5 animate-spin mr-2" />
          Yuklanmoqda...
        </div>
      ) : streams.length === 0 ? (
        <div className="card p-12 flex flex-col items-center justify-center text-center">
          <Radio className="w-12 h-12 text-gray-500 mb-3" />
          <p className="font-medium">Hali stream yo'q</p>
          <p className="text-sm text-gray-400 mt-1">
            Avval bitta kamerali oddiy stream'dan boshlang
          </p>
          <button onClick={() => setEditing(emptyStream)} className="btn-primary mt-4">
            <Plus className="w-4 h-4" />
            Birinchi stream'ni yarating
          </button>
        </div>
      ) : (
        <div className="space-y-3">
          {streams.map((s) => {
            const st = status[s.id];
            const isRunning = st?.state === "running" || st?.state === "starting";
            // Stream key validatsiyasi (faqat non-custom platform uchun)
            const keyValid = s.platform === "custom"
              ? !!s.customUrl
              : validateKey(s.streamKey).ok;
            return (
              <div key={s.id} className="card p-4">
                <div className="flex items-center justify-between gap-4">
                  <div className="flex items-center gap-3 min-w-0">
                    <div className="w-10 h-10 rounded-lg bg-bg-subtle flex items-center justify-center flex-shrink-0">
                      <span className="text-lg">{layoutInfo[s.layout].preview}</span>
                    </div>
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <h3 className="font-semibold truncate">{s.name}</h3>
                        <StateBadge state={st?.state ?? "idle"} />
                        {!keyValid && (
                          <span className="badge-danger text-xs" title="Stream key noto'g'ri">
                            ⚠ Key noto'g'ri
                          </span>
                        )}
                      </div>
                      <p className="text-xs text-gray-400 mt-0.5">
                        {layoutInfo[s.layout].label} • {s.quality} • {s.platform}
                        {st?.state === "running" && ` • ${formatUptime(st.uptime)}`}
                      </p>
                      {((st?.lastError && st.state === "error") ||
                        (startError && startError.streamId === s.id)) && (
                        <ErrorBlock
                          message={st?.lastError || startError?.msg || ""}
                          expanded={expandedError === s.id}
                          onToggle={() => setExpandedError(expandedError === s.id ? null : s.id)}
                          onShowLogs={() => navigate("/logs")}
                        />
                      )}
                    </div>
                  </div>

                  <div className="flex items-center gap-1 flex-shrink-0">
                    {isRunning ? (
                      <button
                        onClick={() => stopMut.mutate(s.id)}
                        disabled={stopMut.isPending}
                        className="btn-danger"
                      >
                        <Square className="w-4 h-4" />
                        To'xtatish
                      </button>
                    ) : !keyValid ? (
                      <button
                        onClick={() => setEditing(s)}
                        className="btn bg-warning/20 text-warning hover:bg-warning/30 border border-warning/30"
                      >
                        <Edit2 className="w-4 h-4" />
                        Key kiriting
                      </button>
                    ) : (
                      <button
                        onClick={() => startMut.mutate(s.id)}
                        disabled={startMut.isPending}
                        className="btn-primary"
                      >
                        <Play className="w-4 h-4" />
                        Ishga tushirish
                      </button>
                    )}
                    <button onClick={() => setEditing(s)} className="btn-ghost p-2">
                      <Edit2 className="w-4 h-4" />
                    </button>
                    <button
                      onClick={() => { if (confirm(`"${s.name}" o'chirilsinmi?`)) delMut.mutate(s.id); }}
                      className="btn-ghost p-2 hover:text-danger"
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {editing && (
        <StreamFormModal
          stream={editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            qc.invalidateQueries({ queryKey: ["streams"] });
          }}
        />
      )}
    </div>
  );
}

// ============================== StreamKeyField ==============================

// Stream key kiritish maydoni — to'liq URL kiritilgan bo'lsa avtomatik tozalaydi
// va foydalanuvchiga oxirgi RTMP URL'ni ko'rsatadi.
const platformBaseUrls: Record<string, string> = {
  youtube: "rtmp://a.rtmp.youtube.com/live2/",
  twitch: "rtmp://live.twitch.tv/app/",
  facebook: "rtmps://live-api-s.facebook.com:443/rtmp/",
};

// Path segmentlari — bularni qabul qilmaymiz (foydalanuvchi xato qilgan ko'rinadi)
const badKeyValues = new Set(["live", "live2", "live3", "app", "rtmp", "stream", "flv", "channel", "ingest"]);

function validateKey(key: string): { ok: boolean; reason?: string } {
  if (!key) return { ok: false, reason: "Bo'sh" };
  if (key.length < 10) return { ok: false, reason: `Juda qisqa (${key.length} belgi, kamida 10 kerak)` };
  if (badKeyValues.has(key.toLowerCase())) {
    return { ok: false, reason: `"${key}" — bu serverning yo'l qismi, stream key emas` };
  }
  return { ok: true };
}

function StreamKeyField({
  platform,
  value,
  onChange,
}: {
  platform: string;
  value: string;
  onChange: (v: string) => void;
}) {
  const baseUrl = platformBaseUrls[platform] || "";

  // Stream key'dan to'liq URL bo'lagini olib tashlaymiz
  const sanitize = (input: string): string => {
    let v = input.trim();
    if (v.startsWith("rtmp://") || v.startsWith("rtmps://")) {
      const lastSlash = v.lastIndexOf("/");
      if (lastSlash !== -1 && lastSlash < v.length - 1) {
        v = v.substring(lastSlash + 1);
      }
    }
    return v;
  };

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    onChange(sanitize(e.target.value));
  };

  const validation = validateKey(value);
  const maskedKey = value.length > 8
    ? value.substring(0, 4) + "•".repeat(value.length - 8) + value.substring(value.length - 4)
    : value;

  return (
    <div>
      <label className="label">Stream Key</label>
      <PasswordInput
        monospace
        placeholder="xxxx-xxxx-xxxx-xxxx-xxxx"
        value={value}
        onChange={handleChange}
      />

      {/* To'liq URL tarjimasi — foydalanuvchi nima ishlatilishini ko'radi */}
      {value && (
        <div className="mt-2 p-2 rounded-md bg-bg-subtle/40 border border-white/5 text-xs font-mono break-all">
          <span className="text-gray-500">→ </span>
          <span className="text-gray-400">{baseUrl}</span>
          <span className={validation.ok ? "text-success" : "text-danger"}>{maskedKey}</span>
        </div>
      )}

      {/* Validatsiya xulosasi */}
      {value && !validation.ok && (
        <div className="mt-2 p-2 rounded-md bg-danger/10 border border-danger/30 text-xs">
          <p className="text-danger font-medium">❌ {validation.reason}</p>
        </div>
      )}
      {value && validation.ok && (
        <p className="mt-1.5 text-xs text-success flex items-center gap-1">
          ✓ Stream key formati to'g'ri ko'rinmoqda
        </p>
      )}

      {/* YouTube uchun aniq instruksiya */}
      {platform === "youtube" && (
        <YouTubeKeyInstructions />
      )}
    </div>
  );
}

function YouTubeKeyInstructions() {
  const [expanded, setExpanded] = useState(false);
  return (
    <div className="mt-3 p-3 rounded-md bg-accent/5 border border-accent/20 text-xs">
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex items-center gap-2 text-accent hover:underline w-full text-left"
      >
        <span>📖</span>
        <span className="font-medium">YouTube Stream key'ni qayerdan olish?</span>
        <span className="ml-auto text-gray-500">{expanded ? "▲" : "▼"}</span>
      </button>
      {expanded && (
        <ol className="mt-3 ml-1 space-y-2 text-gray-300 list-decimal list-inside">
          <li>
            <a
              href="https://studio.youtube.com/channel/UC/livestreaming"
              target="_blank"
              rel="noopener"
              className="text-accent hover:underline"
            >
              YouTube Studio
            </a> ga kiring (yoki <strong>youtube.com</strong> → profilingiz → <strong>YouTube Studio</strong>)
          </li>
          <li>Chap menyudan <strong>"Create"</strong> → <strong>"Go live"</strong> (yoki <strong>"Stream"</strong>) tanlang</li>
          <li>Yangi stream uchun ma'lumotlarni to'ldiring (nom, kategoriya)</li>
          <li>"Stream settings" qismida <strong>"Stream key"</strong> maydonini topasiz</li>
          <li>
            <strong>"COPY"</strong> tugmasini bosing — bu key yangi versiyalarda{" "}
            <code className="bg-bg px-1 py-0.5 rounded">xxxx-xxxx-xxxx-xxxx-xxxx</code> ko'rinishida
          </li>
          <li>Shu yerga paste qiling (Ctrl+V) — <strong>faqat key qismi</strong>, <code>rtmp://</code> emas</li>
        </ol>
      )}
    </div>
  );
}

// ============================== ErrorBlock ==============================

// FFmpeg xatosini ko'p qatorli ko'rsatish + Logs sahifasiga ishora.
function ErrorBlock({
  message,
  expanded,
  onToggle,
  onShowLogs,
}: {
  message: string;
  expanded: boolean;
  onToggle: () => void;
  onShowLogs: () => void;
}) {
  const lines = message.split("\n").filter((l) => l.trim() !== "");
  const summary = lines[0] || "Noma'lum xato";
  const hasDetails = lines.length > 1;

  return (
    <div className="mt-2 p-2 rounded-md bg-danger/10 border border-danger/30 text-xs">
      <div className="flex items-start gap-2">
        <span className="text-danger flex-shrink-0">⚠</span>
        <div className="flex-1 min-w-0">
          <p className="text-danger font-medium break-words">{summary}</p>
          {hasDetails && expanded && (
            <pre className="mt-2 text-gray-300 whitespace-pre-wrap font-mono text-[11px] leading-relaxed max-h-48 overflow-auto">
              {lines.slice(1).join("\n")}
            </pre>
          )}
          <div className="flex items-center gap-3 mt-1.5">
            {hasDetails && (
              <button
                onClick={onToggle}
                className="text-accent hover:underline text-xs"
              >
                {expanded ? "Yashirish" : "Batafsil ko'rsatish"}
              </button>
            )}
            <button
              onClick={onShowLogs}
              className="text-accent hover:underline text-xs flex items-center gap-1"
            >
              <ScrollText className="w-3 h-3" />
              Loglarni ko'rish
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

// ============================== Form Modal ==============================

function StreamFormModal({
  stream,
  onClose,
  onSaved,
}: {
  stream: Partial<Stream>;
  onClose: () => void;
  onSaved: () => void;
}) {
  const camerasQ = useQuery({ queryKey: ["cameras"], queryFn: listCameras });
  const hwQ = useQuery({ queryKey: ["hardware"], queryFn: getHardwareInfo });
  const [form, setForm] = useState<Partial<Stream>>(stream);
  const isNew = !form.id;

  useEscapeKey(onClose);

  const saveMut = useMutation({
    mutationFn: async () => {
      if (isNew) return createStream(form);
      return updateStream(form as Stream);
    },
    onSuccess: onSaved,
  });

  const cameras = camerasQ.data ?? [];
  const needs = layoutInfo[form.layout ?? "single"].needs;
  const cameraIds = form.cameraIds ?? [];

  const setCamera = (slot: number, id: number) => {
    const next = [...cameraIds];
    while (next.length <= slot) next.push(0);
    next[slot] = id;
    setForm({ ...form, cameraIds: next });
  };

  return (
    <div className="fixed inset-0 bg-black/85 backdrop-blur flex items-center justify-center p-4 z-50 animate-in fade-in duration-150" onClick={onClose}>
      <div className="bg-bg-card rounded-xl border border-white/10 shadow-2xl w-full max-w-3xl max-h-[90vh] overflow-auto" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between p-5 border-b border-white/5">
          <h2 className="text-lg font-semibold">{isNew ? "Yangi stream" : "Stream'ni tahrirlash"}</h2>
          <button onClick={onClose} className="btn-ghost p-1"><X className="w-5 h-5" /></button>
        </div>

        <div className="p-5 space-y-5">
          <div>
            <label className="label">Nom</label>
            <input
              className="input"
              placeholder="Masalan: Asosiy YouTube stream"
              value={form.name ?? ""}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
          </div>

          {/* Layout */}
          <div>
            <label className="label">Layout — kameralarni qanday joylashtirish</label>
            <div className="grid grid-cols-3 sm:grid-cols-6 gap-2">
              {(Object.keys(layoutInfo) as LayoutType[]).map((k) => {
                const info = layoutInfo[k];
                const active = form.layout === k;
                return (
                  <button
                    key={k}
                    type="button"
                    onClick={() => setForm({ ...form, layout: k, cameraIds: [] })}
                    className={`p-3 rounded-lg border text-center transition-colors ${
                      active ? "bg-accent/10 border-accent text-accent" : "bg-bg-subtle border-white/5 hover:border-white/20"
                    }`}
                  >
                    <div className="text-2xl">{info.preview}</div>
                    <div className="text-xs mt-1 leading-tight">{info.label}</div>
                  </button>
                );
              })}
            </div>
          </div>

          {/* Kamera tanlash */}
          <div>
            <label className="label">Kameralar ({needs} ta kerak)</label>
            {cameras.length === 0 ? (
              <p className="text-sm text-warning">Avval kamera qo'shing</p>
            ) : (
              <div className="space-y-2">
                {Array.from({ length: needs }).map((_, slot) => (
                  <div key={slot} className="flex items-center gap-2">
                    <span className="text-xs text-gray-500 w-16">
                      {slot === 0 ? "Asosiy" : `${slot + 1}-slot`}
                    </span>
                    <select
                      className="input"
                      value={cameraIds[slot] ?? 0}
                      onChange={(e) => setCamera(slot, Number(e.target.value))}
                    >
                      <option value={0}>— tanlang —</option>
                      {cameras.map((c) => (
                        <option key={c.id} value={c.id}>{c.name} ({c.host})</option>
                      ))}
                    </select>
                  </div>
                ))}
              </div>
            )}
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="label">Sifat</label>
              <select
                className="input"
                value={form.quality}
                onChange={(e) => setForm({ ...form, quality: e.target.value as Quality })}
              >
                <option value="720p30">720p @ 30fps (2.5 Mbps) — tavsiya CPU uchun</option>
                <option value="720p60">720p @ 60fps (4.5 Mbps)</option>
                <option value="1080p30">1080p @ 30fps (4.5 Mbps)</option>
                <option value="1080p60">1080p @ 60fps (6 Mbps)</option>
                <option value="1440p30">1440p @ 30fps (9 Mbps)</option>
              </select>
            </div>
            <div>
              <label className="label">Encoder</label>
              <select
                className="input"
                value={form.encoder}
                onChange={(e) => setForm({ ...form, encoder: e.target.value as Encoder })}
              >
                <option value="auto">Auto — tizimga moslashtiriladi (tavsiya)</option>
                {hwQ.data?.hasNvenc && <option value="h264_nvenc">NVIDIA NVENC (GPU — eng tez)</option>}
                {hwQ.data?.hasQuickSync && <option value="h264_qsv">Intel QuickSync (GPU)</option>}
                {hwQ.data?.hasAmf && <option value="h264_amf">AMD AMF (GPU)</option>}
                {hwQ.data?.hasX264 && <option value="libx264">libx264 (CPU sifatli)</option>}
                {hwQ.data?.hasOpenH264 && <option value="libopenh264">OpenH264 (CPU fallback)</option>}
                <option value="copy">Copy — qayta kodlamaslik (eng yengil)</option>
              </select>
            </div>
          </div>

          {/* CPU foydalanuvchilar uchun maslahat */}
          {hwQ.data && !hwQ.data.hasGpu && (
            <div className="card bg-warning/10 border-warning/30 p-3 text-xs">
              <p className="font-medium text-warning mb-1">⚠️ GPU yo'q — CPU bilan ishlash</p>
              <ul className="space-y-1 text-gray-300 ml-2 list-disc list-inside">
                <li><strong>720p</strong> tavsiya etiladi (1080p CPU'ni qiyinlatadi)</li>
                <li>Encoder: <strong>"Copy"</strong> tanlasa, hech qanday encoding kerakmas (eng yengil)</li>
                <li>Kameralarda <strong>"Sub-stream"</strong> yoqsangiz, kamera o'zi 720p H.264 yuboradi → Copy + Sub-stream = ideal kombinatsiya</li>
              </ul>
            </div>
          )}

          {/* HEVC kameralar uchun tavsiya — sinov natijalari bilan tasdiqlangan */}
          {form.encoder !== "copy" && (
            <div className="card bg-accent/5 border-accent/20 p-3 text-xs">
              <p className="font-medium text-accent mb-1">⚡ Eng barqaror yo'l (sinab ko'rilgan)</p>
              <p className="text-gray-300 mb-2">
                Hikvision <strong>HEVC 1080p</strong> oqimini dekod qilish og'ir → uzilishlar. Sub-stream (640x360) <strong>4-5x yengil</strong> va barqaror.
              </p>
              <ol className="space-y-1 text-gray-300 ml-2 list-decimal list-inside">
                <li><strong>Cameras</strong> → kamerangizni tahrirlash</li>
                <li><strong>"Sub-stream"</strong> ni yoqib saqlang</li>
                <li>Bu yerda <strong>Sifat: "720p30"</strong> tanlang (sub-stream'ga mos)</li>
                <li>Encoder: <strong>"Auto"</strong> (NVENC tanlanadi)</li>
                <li>Natija: <strong>uzluksiz oqim, 0 uzilish</strong></li>
              </ol>
              <p className="text-gray-500 mt-2 text-[11px]">
                💡 Eng yengil variant: kamerada sub-stream H.264 (HEVC emas) ga sozlasangiz, Encoder "Copy" = 0% CPU
              </p>
            </div>
          )}

          <div>
            <label className="label">Audio</label>
            <select
              className="input"
              value={form.audioMode}
              onChange={(e) => setForm({ ...form, audioMode: e.target.value as AudioMode })}
            >
              <option value="first">Birinchi kameradan</option>
              <option value="muted">Audio yo'q (muted)</option>
              <option value="index">Aniq kamera...</option>
            </select>
            {form.audioMode === "index" && (
              <select
                className="input mt-2"
                value={form.audioCameraIndex}
                onChange={(e) => setForm({ ...form, audioCameraIndex: Number(e.target.value) })}
              >
                {Array.from({ length: needs }).map((_, i) => (
                  <option key={i} value={i}>{i === 0 ? "Asosiy" : `${i + 1}-slot`} kamera</option>
                ))}
              </select>
            )}
          </div>

          <hr className="border-white/5" />

          <div>
            <label className="label">Platform</label>
            <select
              className="input"
              value={form.platform}
              onChange={(e) => setForm({ ...form, platform: e.target.value as Platform })}
            >
              <option value="youtube">YouTube Live</option>
              <option value="twitch">Twitch</option>
              <option value="facebook">Facebook Live</option>
              <option value="custom">Custom RTMP</option>
            </select>
          </div>

          {form.platform === "custom" ? (
            <div>
              <label className="label">RTMP URL (stream key bilan)</label>
              <PasswordInput
                monospace
                placeholder="rtmp://server/app/STREAM_KEY"
                value={form.customUrl ?? ""}
                onChange={(e) => setForm({ ...form, customUrl: e.target.value })}
              />
            </div>
          ) : (
            <StreamKeyField
              platform={form.platform || "youtube"}
              value={form.streamKey ?? ""}
              onChange={(v) => setForm({ ...form, streamKey: v })}
            />
          )}

          {/* Auto-restart */}
          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                id="autoRestart"
                checked={form.autoRestart ?? false}
                onChange={(e) => setForm({ ...form, autoRestart: e.target.checked })}
                className="w-4 h-4"
              />
              <label htmlFor="autoRestart" className="text-sm">
                Uzilsa avtomatik qayta urinish
              </label>
            </div>
          </div>

          {saveMut.error && (
            <div className="text-sm text-danger">{String(saveMut.error)}</div>
          )}
        </div>

        <div className="flex justify-end gap-2 p-5 border-t border-white/5">
          <button onClick={onClose} className="btn-secondary">Bekor qilish</button>
          <button
            onClick={() => saveMut.mutate()}
            disabled={!form.name || saveMut.isPending}
            className="btn-primary"
          >
            {saveMut.isPending ? <Loader2 className="w-4 h-4 animate-spin" /> : null}
            Saqlash
          </button>
        </div>
      </div>
    </div>
  );
}
