import { useEffect, useRef, useState } from "react";
import Hls from "hls.js";
import { X, Loader2, AlertTriangle, RefreshCw } from "lucide-react";
import { startPreview, stopPreview } from "../lib/api";
import { useEscapeKey } from "../lib/hooks";
import type { Camera } from "../lib/types";

/**
 * PreviewModal — kamerani jonli ko'rish (HLS player).
 *
 * Oqim:
 *  1. Modal ochilgan zahoti backend'ga StartPreview(cameraId) yuboriladi
 *  2. Backend FFmpeg ishga tushiradi, HLS segmentlarini yozadi
 *  3. hls.js /preview/{id}/index.m3u8 ni 1-2 sekund kutib, ijro etadi
 *  4. Modal yopilganda StopPreview chaqiriladi va process to'xtatiladi
 *
 * Latentlik taxminan 3-5 soniya — HLS ning normal xususiyati. Real-time emas,
 * lekin kamera holatini kuzatish uchun mukammal.
 */
export function PreviewModal({
  camera,
  onClose,
}: {
  camera: Camera;
  onClose: () => void;
}) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const hlsRef = useRef<Hls | null>(null);
  const [state, setState] = useState<"starting" | "loading" | "playing" | "error">("starting");
  const [errorMsg, setErrorMsg] = useState<string>("");

  useEscapeKey(onClose);

  // Backend preview'ni boshlash va m3u8 fayli yaratilishini kutish
  useEffect(() => {
    let mounted = true;
    let retryTimer: ReturnType<typeof setTimeout> | null = null;

    (async () => {
      try {
        setState("starting");
        await startPreview(camera.id);
        if (!mounted) return;

        // FFmpeg birinchi segmentni yozishi uchun 1-2 sekund kutamiz
        retryTimer = setTimeout(() => {
          if (mounted) setupHls();
        }, 1500);
      } catch (err) {
        if (mounted) {
          setState("error");
          setErrorMsg(String(err));
        }
      }
    })();

    return () => {
      mounted = false;
      if (retryTimer) clearTimeout(retryTimer);
      if (hlsRef.current) {
        hlsRef.current.destroy();
        hlsRef.current = null;
      }
      // Backend preview'ni to'xtatamiz (fonda)
      void stopPreview(camera.id);
    };
  }, [camera.id]);

  const setupHls = () => {
    if (!videoRef.current) return;
    const video = videoRef.current;
    const src = `/preview/${camera.id}/index.m3u8`;

    if (Hls.isSupported()) {
      const hls = new Hls({
        // Live stream uchun maxsus sozlamalar — past latentlik, kichik buffer
        liveSyncDurationCount: 2,
        liveMaxLatencyDurationCount: 5,
        maxBufferLength: 5,
        enableWorker: true,
      });
      hlsRef.current = hls;

      hls.on(Hls.Events.MEDIA_ATTACHED, () => {
        setState("loading");
      });
      hls.on(Hls.Events.MANIFEST_PARSED, () => {
        video.play().catch(() => {/* autoplay block — user gesture bilan ochiladi */});
        setState("playing");
      });
      hls.on(Hls.Events.ERROR, (_, data) => {
        if (data.fatal) {
          switch (data.type) {
            case Hls.ErrorTypes.NETWORK_ERROR:
              // Boshlanishida m3u8 hali yo'q — biroz kutib qayta urinish
              setTimeout(() => hls.startLoad(), 800);
              break;
            case Hls.ErrorTypes.MEDIA_ERROR:
              hls.recoverMediaError();
              break;
            default:
              setState("error");
              setErrorMsg(`HLS: ${data.details}`);
              break;
          }
        }
      });

      hls.loadSource(src);
      hls.attachMedia(video);
    } else if (video.canPlayType("application/vnd.apple.mpegurl")) {
      // Safari — native HLS
      video.src = src;
      video.addEventListener("loadedmetadata", () => {
        void video.play();
        setState("playing");
      });
    } else {
      setState("error");
      setErrorMsg("Bu brauzerda HLS qo'llab-quvvatlanmaydi");
    }
  };

  const handleRetry = () => {
    setState("starting");
    setErrorMsg("");
    void startPreview(camera.id).then(() => {
      setTimeout(setupHls, 1500);
    }).catch((e) => {
      setState("error");
      setErrorMsg(String(e));
    });
  };

  return (
    <div
      className="fixed inset-0 bg-black/90 backdrop-blur flex items-center justify-center p-4 z-50 animate-in fade-in duration-150"
      onClick={onClose}
    >
      <div
        className="bg-bg-card rounded-xl border border-white/10 shadow-2xl w-full max-w-4xl overflow-hidden"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between p-4 border-b border-white/10">
          <div className="min-w-0">
            <h2 className="text-lg font-semibold truncate">{camera.name}</h2>
            <p className="text-xs text-gray-500 mt-0.5 truncate">
              {camera.vendor} • {camera.host} • {camera.useSubStream ? "sub-stream" : "main stream"}
            </p>
          </div>
          <button onClick={onClose} className="btn-ghost p-1.5 flex-shrink-0" title="Yopish (Esc)">
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Video maydoni */}
        <div className="relative bg-black aspect-video">
          <video
            ref={videoRef}
            className="w-full h-full object-contain"
            controls
            muted // autoplay uchun majburiy
            playsInline
          />

          {/* Status overlay */}
          {state !== "playing" && (
            <div className="absolute inset-0 flex flex-col items-center justify-center bg-black/40 backdrop-blur-sm">
              {state === "starting" && (
                <>
                  <Loader2 className="w-8 h-8 animate-spin text-accent" />
                  <p className="mt-3 text-sm text-gray-300">Kameraga ulanmoqda...</p>
                </>
              )}
              {state === "loading" && (
                <>
                  <Loader2 className="w-8 h-8 animate-spin text-accent" />
                  <p className="mt-3 text-sm text-gray-300">Video oqimi yuklanmoqda...</p>
                  <p className="mt-1 text-xs text-gray-500">Birinchi marta ~3 sekund kutiladi</p>
                </>
              )}
              {state === "error" && (
                <>
                  <AlertTriangle className="w-8 h-8 text-danger" />
                  <p className="mt-3 text-sm font-medium text-danger">Ulanishda xatolik</p>
                  <p className="mt-1 text-xs text-gray-400 max-w-md text-center px-4">{errorMsg}</p>
                  <button onClick={handleRetry} className="btn-secondary mt-4">
                    <RefreshCw className="w-4 h-4" />
                    Qayta urinish
                  </button>
                </>
              )}
            </div>
          )}
        </div>

        {/* Pastki info */}
        <div className="p-3 text-xs text-gray-500 bg-bg-subtle/30">
          <p>
            ℹ️ Latentlik: ~3-5 sek (HLS xususiyati). Audio ko'rsatilmaydi. Stream'lash ishi alohida ishlaydi va shunga ta'sir qilmaydi.
          </p>
        </div>
      </div>
    </div>
  );
}
