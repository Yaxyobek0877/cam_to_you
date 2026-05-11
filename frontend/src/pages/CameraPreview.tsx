import { useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import Hls from "hls.js";
import {
  ArrowLeft, Loader2, AlertTriangle, RefreshCw, Camera as CameraIcon,
  Maximize2, Volume2, VolumeX, Info, Activity, Terminal, ChevronDown, ChevronUp,
} from "lucide-react";
import { getCamera, startPreview, stopPreview, onPreviewEvent } from "../lib/api";
import { useEscapeKey } from "../lib/hooks";

/**
 * CameraPreview — to'liq sahifa kamera live preview.
 *
 * URL: /cameras/:id/preview
 *
 * Xususiyatlar:
 *   - To'liq sahifa video player
 *   - Real-time FFmpeg log oqimi (pastda kengaytiriladigan panel)
 *   - Encoder, RTSP URL va boshqa diagnostic ma'lumot
 *   - Tola ekran rejimi
 *   - Escape yoki Back → kameralar ro'yxatiga qaytish
 */
type PreviewState = "starting" | "loading" | "playing" | "error";

interface PreviewLogLine {
  time: Date;
  level: "info" | "warning" | "error";
  message: string;
}

export function CameraPreview() {
  const { id } = useParams();
  const cameraId = Number(id);
  const navigate = useNavigate();

  const cameraQ = useQuery({
    queryKey: ["camera", cameraId],
    queryFn: () => getCamera(cameraId),
    enabled: !isNaN(cameraId),
  });

  const videoRef = useRef<HTMLVideoElement>(null);
  const hlsRef = useRef<Hls | null>(null);
  const [state, setState] = useState<PreviewState>("starting");
  const [errorMsg, setErrorMsg] = useState<string>("");
  const [logs, setLogs] = useState<PreviewLogLine[]>([]);
  const [muted, setMuted] = useState(true);
  const [showLogs, setShowLogs] = useState(true);
  const [encoder, setEncoder] = useState<string>("");
  const [rtspMasked, setRtspMasked] = useState<string>("");

  const goBack = () => navigate("/cameras");
  useEscapeKey(goBack);

  // Preview start + event listening
  useEffect(() => {
    if (isNaN(cameraId)) return;
    let mounted = true;
    let hlsStarted = false;

    // Preview hodisalarini eshitamiz
    const unsub = onPreviewEvent((data: any) => {
      if (!mounted || data?.cameraId !== cameraId) return;

      switch (data.type) {
        case "starting":
          setEncoder(data.payload?.encoder || "");
          setRtspMasked(data.payload?.rtspUrl || "");
          appendLog("info", `Preview ishga tushdi (encoder: ${data.payload?.encoder})`);
          break;
        case "ready":
          appendLog("info", "✓ Birinchi video segment tayyor — player ulanmoqda");
          if (!hlsStarted) {
            hlsStarted = true;
            setupHls();
          }
          break;
        case "log":
          appendLog(
            data.payload?.level || "info",
            data.payload?.message || "",
          );
          break;
        case "error":
          const errStr = typeof data.payload === "string" ? data.payload : JSON.stringify(data.payload);
          appendLog("error", errStr);
          setState("error");
          setErrorMsg(errStr);
          break;
        case "stopped":
          appendLog("info", "Preview to'xtatildi");
          break;
      }
    });

    // Boshlash — fixed timer YO'Q, biz backend'dan "ready" event'ni kutamiz.
    // Bu kameraga ulanish vaqti har xil bo'lganda yaxshiroq ishlaydi.
    (async () => {
      try {
        setState("starting");
        appendLog("info", "Backend'ga StartPreview yuborilmoqda...");
        await startPreview(cameraId);
        if (!mounted) return;
        setState("loading");
        appendLog("info", "FFmpeg ishlayotgani kutilmoqda...");
      } catch (err) {
        if (mounted) {
          const msg = String(err);
          setState("error");
          setErrorMsg(msg);
          appendLog("error", msg);
        }
      }
    })();

    return () => {
      mounted = false;
      if (hlsRef.current) {
        hlsRef.current.destroy();
        hlsRef.current = null;
      }
      void stopPreview(cameraId);
      unsub?.();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cameraId]);

  const appendLog = (level: PreviewLogLine["level"], message: string) => {
    setLogs((prev) => {
      const next = [...prev, { time: new Date(), level, message }];
      if (next.length > 200) return next.slice(next.length - 200);
      return next;
    });
  };

  const setupHls = () => {
    if (!videoRef.current) return;
    const video = videoRef.current;
    const src = `/preview/${cameraId}/index.m3u8`;

    if (hlsRef.current) {
      hlsRef.current.destroy();
    }

    if (Hls.isSupported()) {
      const hls = new Hls({
        liveSyncDurationCount: 2,
        liveMaxLatencyDurationCount: 5,
        maxBufferLength: 5,
        enableWorker: true,
      });
      hlsRef.current = hls;

      hls.on(Hls.Events.MANIFEST_PARSED, () => {
        appendLog("info", "HLS manifest yuklandi, video ijro qilinmoqda");
        video.play().catch(() => {});
        setState("playing");
      });
      hls.on(Hls.Events.ERROR, (_, data) => {
        if (data.fatal) {
          appendLog("warning", `HLS xato: ${data.details}, qayta urinilmoqda...`);
          switch (data.type) {
            case Hls.ErrorTypes.NETWORK_ERROR:
              setTimeout(() => hls.startLoad(), 800);
              break;
            case Hls.ErrorTypes.MEDIA_ERROR:
              hls.recoverMediaError();
              break;
          }
        }
      });

      hls.loadSource(src);
      hls.attachMedia(video);
    } else if (video.canPlayType("application/vnd.apple.mpegurl")) {
      video.src = src;
      video.addEventListener("loadedmetadata", () => {
        void video.play();
        setState("playing");
      });
    } else {
      setState("error");
      setErrorMsg("Brauzer HLS qo'llab-quvvatlamaydi");
    }
  };

  const handleRetry = async () => {
    setLogs([]);
    setState("starting");
    setErrorMsg("");
    appendLog("info", "Qayta ulanish...");
    try {
      await startPreview(cameraId);
      setTimeout(setupHls, 1500);
    } catch (e) {
      setState("error");
      setErrorMsg(String(e));
      appendLog("error", String(e));
    }
  };

  const toggleFullscreen = () => {
    if (videoRef.current) {
      videoRef.current.requestFullscreen?.();
    }
  };

  if (cameraQ.isLoading) {
    return (
      <div className="p-6 flex items-center justify-center h-full">
        <Loader2 className="w-6 h-6 animate-spin text-gray-500" />
      </div>
    );
  }

  if (cameraQ.isError || !cameraQ.data) {
    return (
      <div className="p-6">
        <button onClick={goBack} className="btn-secondary mb-4">
          <ArrowLeft className="w-4 h-4" />
          Kameralarga qaytish
        </button>
        <div className="card p-6 text-center">
          <AlertTriangle className="w-12 h-12 text-danger mx-auto mb-3" />
          <p className="font-medium">Kamera topilmadi</p>
          <p className="text-sm text-gray-400 mt-1">ID: {cameraId}</p>
        </div>
      </div>
    );
  }

  const camera = cameraQ.data;

  return (
    <div className="flex flex-col h-full max-h-screen overflow-hidden">
      {/* Header */}
      <header className="flex-shrink-0 px-6 py-4 border-b border-white/5 flex items-center justify-between">
        <div className="flex items-center gap-3 min-w-0">
          <button onClick={goBack} className="btn-ghost p-2 flex-shrink-0" title="Orqaga (Esc)">
            <ArrowLeft className="w-5 h-5" />
          </button>
          <div className="min-w-0">
            <h1 className="text-xl font-bold flex items-center gap-2">
              <CameraIcon className="w-5 h-5" />
              {camera.name}
            </h1>
            <p className="text-xs text-gray-500 mt-0.5 truncate">
              {camera.vendor} • {camera.host}:{camera.port} • channel {camera.channel}
              {camera.useSubStream ? " (sub)" : " (main)"}
              {encoder && ` • encoder: ${encoder}`}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2 flex-shrink-0">
          <button onClick={() => setMuted(!muted)} className="btn-ghost p-2" title={muted ? "Audio yoqish" : "Audio o'chirish"}>
            {muted ? <VolumeX className="w-4 h-4" /> : <Volume2 className="w-4 h-4" />}
          </button>
          <button onClick={toggleFullscreen} className="btn-ghost p-2" title="Tola ekran">
            <Maximize2 className="w-4 h-4" />
          </button>
          {state === "error" && (
            <button onClick={handleRetry} className="btn-primary">
              <RefreshCw className="w-4 h-4" />
              Qayta urinish
            </button>
          )}
        </div>
      </header>

      {/* Asosiy maydon */}
      <div className="flex-1 flex flex-col min-h-0">
        {/* Video */}
        <div className="relative flex-1 bg-black flex items-center justify-center min-h-0">
          <video
            ref={videoRef}
            className="max-w-full max-h-full"
            controls={state === "playing"}
            muted={muted}
            playsInline
          />

          {state !== "playing" && (
            <div className="absolute inset-0 flex flex-col items-center justify-center bg-black/60 backdrop-blur-sm">
              {state === "starting" && (
                <>
                  <Loader2 className="w-10 h-10 animate-spin text-accent" />
                  <p className="mt-4 text-base font-medium">Kameraga ulanmoqda...</p>
                  <p className="mt-1 text-sm text-gray-500">FFmpeg ishga tushirilmoqda</p>
                </>
              )}
              {state === "loading" && (
                <>
                  <Loader2 className="w-10 h-10 animate-spin text-accent" />
                  <p className="mt-4 text-base font-medium">Video oqimi yaratilmoqda...</p>
                  <p className="mt-1 text-sm text-gray-500">Birinchi marta ~3-5 sekund kutiladi</p>
                  <p className="mt-3 text-xs text-gray-600">
                    Agar uzoq cho'zilsa, pastda <strong>FFmpeg loglari</strong>ni ko'ring
                  </p>
                </>
              )}
              {state === "error" && (
                <>
                  <AlertTriangle className="w-12 h-12 text-danger" />
                  <p className="mt-4 text-base font-medium text-danger">Ulanishda xatolik</p>
                  <p className="mt-1 text-xs text-gray-400 max-w-xl text-center px-4 break-words">{errorMsg}</p>
                  <button onClick={handleRetry} className="btn-primary mt-5">
                    <RefreshCw className="w-4 h-4" />
                    Qayta urinish
                  </button>
                </>
              )}
            </div>
          )}
        </div>

        {/* Pastki diagnostic panel */}
        <div className="flex-shrink-0 border-t border-white/5">
          <button
            onClick={() => setShowLogs(!showLogs)}
            className="w-full px-6 py-2 flex items-center justify-between hover:bg-bg-subtle/40 transition-colors"
          >
            <span className="flex items-center gap-2 text-sm font-medium">
              <Terminal className="w-4 h-4" />
              FFmpeg loglari ({logs.length})
            </span>
            {showLogs ? <ChevronDown className="w-4 h-4 text-gray-500" /> : <ChevronUp className="w-4 h-4 text-gray-500" />}
          </button>

          {showLogs && (
            <div className="px-6 pb-4">
              {/* Diagnostic ma'lumot */}
              <div className="grid grid-cols-2 md:grid-cols-4 gap-3 mb-3 text-xs">
                <DiagItem icon={Activity} label="Holat" value={state} />
                <DiagItem icon={Info} label="Encoder" value={encoder || "..."} />
                <DiagItem icon={Info} label="RTSP" value={rtspMasked || "..."} mono />
                <DiagItem icon={Info} label="Loglar" value={`${logs.length} qator`} />
              </div>

              {/* Log oqimi */}
              <div className="bg-bg max-h-64 overflow-y-auto rounded-lg border border-white/5 p-2 font-mono text-xs leading-relaxed">
                {logs.length === 0 ? (
                  <div className="text-gray-600 text-center py-4">Hali log yo'q</div>
                ) : (
                  logs.map((l, i) => (
                    <div key={i} className="flex gap-2 py-0.5">
                      <span className="text-gray-600 tabular-nums flex-shrink-0">
                        {l.time.toLocaleTimeString("uz-UZ", { hour12: false })}
                      </span>
                      <span className={
                        l.level === "error" ? "text-danger" :
                        l.level === "warning" ? "text-warning" :
                        "text-gray-300"
                      }>
                        {l.message}
                      </span>
                    </div>
                  ))
                )}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function DiagItem({
  icon: Icon, label, value, mono = false,
}: {
  icon: React.ElementType; label: string; value: string; mono?: boolean;
}) {
  return (
    <div className="flex items-center gap-2 bg-bg-subtle rounded-lg p-2">
      <Icon className="w-4 h-4 text-gray-500 flex-shrink-0" />
      <div className="min-w-0">
        <div className="text-xs text-gray-500">{label}</div>
        <div className={`text-xs text-gray-200 truncate ${mono ? "font-mono" : ""}`} title={value}>
          {value}
        </div>
      </div>
    </div>
  );
}
