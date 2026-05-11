import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, AlertCircle, Download, Loader2, Cpu, Activity, FolderOpen, X, Gauge } from "lucide-react";
import { getFFmpegStatus, installFFmpeg, browseFFmpegFile, onFFmpegProgress } from "../lib/api";
import type { FFmpegProgress } from "../lib/types";

export function Settings() {
  const qc = useQueryClient();
  const ffmpegQ = useQuery({ queryKey: ["ffmpeg"], queryFn: getFFmpegStatus });
  const [progress, setProgress] = useState<FFmpegProgress | null>(null);

  useEffect(() => {
    const unsub = onFFmpegProgress((data) => setProgress(data as FFmpegProgress));
    return () => {
      if (typeof unsub === "function") (unsub as any)();
    };
  }, []);

  const installMut = useMutation({
    mutationFn: installFFmpeg,
    onSuccess: () => {
      setProgress(null);
      qc.invalidateQueries({ queryKey: ["ffmpeg"] });
    },
    onError: (e) => alert(`O'rnatish xatoligi: ${e}`),
  });

  const browseMut = useMutation({
    mutationFn: browseFFmpegFile,
    onSuccess: () => {
      setProgress(null);
      qc.invalidateQueries({ queryKey: ["ffmpeg"] });
    },
    onError: (e) => alert(`O'rnatish xatoligi: ${e}`),
  });

  const installing = installMut.isPending || browseMut.isPending;

  return (
    <div className="p-6 space-y-6 max-w-3xl">
      <header>
        <h1 className="text-2xl font-bold">Sozlamalar</h1>
        <p className="text-sm text-gray-400 mt-1">Dastur va FFmpeg sozlamalari</p>
      </header>

      {/* FFmpeg karta */}
      <section className="card p-5">
        <h2 className="font-semibold flex items-center gap-2">
          <Cpu className="w-5 h-5" />
          FFmpeg
        </h2>
        <p className="text-sm text-gray-400 mt-1">
          Stream'lar uchun zarur. Dastur papkasiga (%APPDATA%\Cam2You\bin) o'rnatiladi.
        </p>

        <div className="mt-4">
          {ffmpegQ.isLoading ? (
            <div className="flex items-center gap-2 text-gray-400">
              <Loader2 className="w-4 h-4 animate-spin" />
              Tekshirilmoqda...
            </div>
          ) : ffmpegQ.data?.installed ? (
            <div className="space-y-2">
              <div className="flex items-center gap-2 text-success">
                <CheckCircle2 className="w-5 h-5" />
                <span className="font-medium">O'rnatilgan</span>
              </div>
              <div className="space-y-1 text-xs text-gray-400 ml-7">
                <p>
                  Versiya: <span className="text-gray-200">{ffmpegQ.data.version || "noma'lum"}</span>
                </p>
                <p className="font-mono break-all">Yo'l: {ffmpegQ.data.path}</p>
              </div>
              <div className="ml-7 mt-3">
                <button
                  onClick={() => browseMut.mutate()}
                  disabled={installing}
                  className="btn-secondary text-xs"
                  title="Boshqa versiya bilan almashtirish"
                >
                  <FolderOpen className="w-3.5 h-3.5" />
                  Boshqa fayl bilan almashtirish
                </button>
              </div>
            </div>
          ) : (
            <div className="space-y-3">
              <div className="flex items-center gap-2 text-warning">
                <AlertCircle className="w-5 h-5" />
                <span className="font-medium">O'rnatilmagan</span>
              </div>

              {installing && progress && (
                <ProgressView progress={progress} />
              )}

              {!installing && (
                <div className="space-y-3">
                  <button
                    onClick={() => installMut.mutate()}
                    className="btn-primary"
                  >
                    <Download className="w-4 h-4" />
                    Avtomatik yuklash va o'rnatish
                  </button>

                  <div className="flex items-center gap-2 my-3">
                    <div className="flex-1 h-px bg-white/5" />
                    <span className="text-xs text-gray-500">YOKI</span>
                    <div className="flex-1 h-px bg-white/5" />
                  </div>

                  <div className="space-y-2">
                    <p className="text-xs text-gray-400">
                      Agar avtomatik yuklash sekin bo'lsa, FFmpeg'ni boshqa joydan yuklang va shu yerda ko'rsating:
                    </p>
                    <div className="flex items-center gap-2">
                      <button onClick={() => browseMut.mutate()} className="btn-secondary">
                        <FolderOpen className="w-4 h-4" />
                        Mavjud fayldan o'rnatish (.exe yoki .zip)
                      </button>
                    </div>
                    <details className="text-xs text-gray-500 mt-2">
                      <summary className="cursor-pointer hover:text-gray-300">
                        Qaerdan yuklash mumkin?
                      </summary>
                      <div className="mt-2 ml-3 space-y-1">
                        <p>
                          🔗{" "}
                          <a
                            href="https://www.gyan.dev/ffmpeg/builds/"
                            target="_blank"
                            rel="noopener"
                            className="text-accent hover:underline"
                          >
                            gyan.dev/ffmpeg/builds
                          </a>
                          {" "}— "release essentials" (~80 MB)
                        </p>
                        <p>
                          🔗{" "}
                          <a
                            href="https://github.com/BtbN/FFmpeg-Builds/releases"
                            target="_blank"
                            rel="noopener"
                            className="text-accent hover:underline"
                          >
                            github.com/BtbN/FFmpeg-Builds
                          </a>
                          {" "}— GitHub'dan, GPL/LGPL variantlar
                        </p>
                        <p className="text-gray-600 italic mt-2">
                          Yuklab olganingizdan keyin, .zip ni shu yerda ko'rsating yoki .exe ni alohida olib kelsangiz ham bo'ladi.
                        </p>
                      </div>
                    </details>
                  </div>
                </div>
              )}
            </div>
          )}
        </div>
      </section>

      {/* Foydali ma'lumotlar */}
      <section className="card p-5">
        <h2 className="font-semibold flex items-center gap-2">
          <Activity className="w-5 h-5" />
          Ma'lumot
        </h2>
        <div className="mt-3 space-y-2 text-sm text-gray-400">
          <p>
            Dasturni "X" tugmasi bilan yopsangiz, <strong>tray'ga yashiriladi</strong> — streamlar to'xtamaydi.
          </p>
          <p>
            Butunlay yopish uchun chap pastdagi <strong>Chiqish</strong> tugmasini bosing yoki tray menyusidan tanlang.
          </p>
          <p>
            Ekran o'chsa ham streamlar ishlashda davom etadi — tizim uxlashi avtomatik bloklanadi.
          </p>
        </div>
      </section>
    </div>
  );
}

// ProgressView — yuklash jarayoni ko'rinishi: foiz, tezlik, ETA
function ProgressView({ progress }: { progress: FFmpegProgress }) {
  const stage =
    progress.stage === "downloading" ? "Yuklab olinmoqda" :
    progress.stage === "extracting" ? "Chiqarilmoqda" :
    "Tugallandi";

  return (
    <div className="space-y-2 bg-bg-subtle/30 p-3 rounded-lg">
      <div className="flex items-center justify-between text-xs">
        <span className="text-gray-300">{stage}</span>
        {progress.percent >= 0 && (
          <span className="text-gray-200 font-mono">{progress.percent.toFixed(0)}%</span>
        )}
      </div>

      <div className="w-full h-2 bg-bg rounded-full overflow-hidden">
        <div
          className="h-full bg-accent transition-all duration-300"
          style={{ width: `${Math.max(2, progress.percent)}%` }}
        />
      </div>

      <div className="flex items-center justify-between text-xs">
        <span className="text-gray-500">{progress.message}</span>
        <div className="flex items-center gap-3 text-gray-400">
          {progress.speedMBps > 0 && (
            <span className="flex items-center gap-1">
              <Gauge className="w-3 h-3" />
              {progress.speedMBps.toFixed(2)} MB/s
            </span>
          )}
          {progress.etaSec > 0 && (
            <span>~ {formatEta(progress.etaSec)} qoldi</span>
          )}
        </div>
      </div>
    </div>
  );
}

function formatEta(sec: number): string {
  if (sec < 60) return `${sec} sek`;
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  if (m < 60) return `${m} daq ${s} sek`;
  const h = Math.floor(m / 60);
  return `${h} soat ${m % 60} daq`;
}
