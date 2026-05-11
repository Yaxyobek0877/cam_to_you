import { useQuery } from "@tanstack/react-query";
import { Activity, Camera, Radio, AlertCircle, CheckCircle2, Loader2 } from "lucide-react";
import { Link } from "react-router-dom";
import { listCameras, listStreams, getAllStreamStatus, getFFmpegStatus } from "../lib/api";
import { formatUptime, cn } from "../lib/utils";
import type { StreamState } from "../lib/types";

export function Dashboard() {
  // Asosiy ma'lumotlar
  const camerasQ = useQuery({ queryKey: ["cameras"], queryFn: listCameras });
  const streamsQ = useQuery({ queryKey: ["streams"], queryFn: listStreams });
  const statusQ = useQuery({
    queryKey: ["allStatus"],
    queryFn: getAllStreamStatus,
    refetchInterval: 2_000, // har 2 sekundda yangilanadi
  });
  const ffmpegQ = useQuery({ queryKey: ["ffmpeg"], queryFn: getFFmpegStatus });

  const cameras = camerasQ.data ?? [];
  const streams = streamsQ.data ?? [];
  const status = statusQ.data ?? {};

  const runningCount = Object.values(status).filter(
    (s) => s.state === "running" || s.state === "starting",
  ).length;
  const errorCount = Object.values(status).filter((s) => s.state === "error").length;

  return (
    <div className="p-6 space-y-6">
      <header>
        <h1 className="text-2xl font-bold">Boshqaruv paneli</h1>
        <p className="text-sm text-gray-400 mt-1">
          Tizim umumiy holati va aktiv streamlar
        </p>
      </header>

      {/* FFmpeg ogohlantirish */}
      {ffmpegQ.data && !ffmpegQ.data.installed && (
        <div className="card p-4 bg-warning/10 border-warning/30">
          <div className="flex items-start gap-3">
            <AlertCircle className="w-5 h-5 text-warning flex-shrink-0 mt-0.5" />
            <div className="flex-1">
              <h3 className="font-medium text-warning">FFmpeg o'rnatilmagan</h3>
              <p className="text-sm text-gray-300 mt-1">
                Streamlash ishlashi uchun FFmpeg kerak.{" "}
                <Link to="/settings" className="text-accent hover:underline">
                  Sozlamalardan o'rnating →
                </Link>
              </p>
            </div>
          </div>
        </div>
      )}

      {/* Statistika kartalari */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard
          icon={Camera}
          label="Kameralar"
          value={cameras.length}
          to="/cameras"
        />
        <StatCard
          icon={Radio}
          label="Saqlangan streamlar"
          value={streams.length}
          to="/streams"
        />
        <StatCard
          icon={Activity}
          label="Aktiv stream"
          value={runningCount}
          color="success"
          to="/streams"
        />
        <StatCard
          icon={AlertCircle}
          label="Xato holatda"
          value={errorCount}
          color={errorCount > 0 ? "danger" : "muted"}
          to="/streams"
        />
      </div>

      {/* Aktiv streamlar */}
      <section className="card p-5">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold">Aktiv streamlar</h2>
          <Link to="/streams" className="text-sm text-accent hover:underline">
            Hammasini ko'rish →
          </Link>
        </div>

        {streamsQ.isLoading ? (
          <div className="flex items-center justify-center py-8 text-gray-500">
            <Loader2 className="w-5 h-5 animate-spin mr-2" />
            Yuklanmoqda...
          </div>
        ) : streams.length === 0 ? (
          <EmptyState
            icon={Radio}
            title="Hali stream yo'q"
            description="Boshlash uchun avval kamera qo'shing, keyin stream yarating"
            cta={{ to: "/streams", label: "Stream yaratish" }}
          />
        ) : (
          <div className="space-y-2">
            {streams.map((s) => {
              const st = status[s.id];
              return (
                <div
                  key={s.id}
                  className="flex items-center justify-between p-3 rounded-lg bg-bg-subtle hover:bg-bg/50 transition-colors"
                >
                  <div className="flex items-center gap-3 min-w-0">
                    <StateBadge state={st?.state ?? "idle"} />
                    <div className="min-w-0">
                      <p className="font-medium truncate">{s.name}</p>
                      <p className="text-xs text-gray-500">
                        {s.layout} • {s.quality} • {s.platform}
                      </p>
                    </div>
                  </div>
                  <div className="text-right text-xs text-gray-400">
                    {st && st.state === "running" && (
                      <span>{formatUptime(st.uptime)}</span>
                    )}
                    {st && st.state === "error" && (
                      <span className="text-danger truncate max-w-[200px] inline-block">
                        {st.lastError}
                      </span>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </section>
    </div>
  );
}

// ---- Yordamchi komponentlar ----

function StatCard({
  icon: Icon,
  label,
  value,
  color = "info",
  to,
}: {
  icon: React.ElementType;
  label: string;
  value: number | string;
  color?: "info" | "success" | "warning" | "danger" | "muted";
  to?: string;
}) {
  const colors = {
    info: "text-accent bg-accent/10",
    success: "text-success bg-success/10",
    warning: "text-warning bg-warning/10",
    danger: "text-danger bg-danger/10",
    muted: "text-gray-400 bg-white/5",
  };
  const content = (
    <div className="card p-4 hover:border-white/10 transition-colors">
      <div className="flex items-center gap-3">
        <div className={cn("w-10 h-10 rounded-lg flex items-center justify-center", colors[color])}>
          <Icon className="w-5 h-5" />
        </div>
        <div>
          <p className="text-xs text-gray-400">{label}</p>
          <p className="text-2xl font-bold">{value}</p>
        </div>
      </div>
    </div>
  );
  if (to) return <Link to={to}>{content}</Link>;
  return content;
}

export function StateBadge({ state }: { state: StreamState }) {
  const map: Record<StreamState, { className: string; label: string; icon?: React.ElementType }> = {
    idle: { className: "badge-muted", label: "Bo'sh" },
    starting: { className: "badge-info", label: "Ishga tushmoqda", icon: Loader2 },
    running: { className: "badge-success", label: "Ishlamoqda", icon: CheckCircle2 },
    stopping: { className: "badge-warning", label: "To'xtamoqda", icon: Loader2 },
    stopped: { className: "badge-muted", label: "To'xtatilgan" },
    error: { className: "badge-danger", label: "Xato", icon: AlertCircle },
  };
  const info = map[state] ?? map.idle;
  const Icon = info.icon;
  return (
    <span className={info.className}>
      {Icon && <Icon className={cn("w-3 h-3", (state === "starting" || state === "stopping") && "animate-spin")} />}
      {info.label}
    </span>
  );
}

function EmptyState({
  icon: Icon,
  title,
  description,
  cta,
}: {
  icon: React.ElementType;
  title: string;
  description: string;
  cta?: { to: string; label: string };
}) {
  return (
    <div className="flex flex-col items-center justify-center py-12 text-center">
      <div className="w-12 h-12 rounded-full bg-bg-subtle flex items-center justify-center mb-3">
        <Icon className="w-6 h-6 text-gray-500" />
      </div>
      <p className="font-medium">{title}</p>
      <p className="text-sm text-gray-400 mt-1 max-w-sm">{description}</p>
      {cta && (
        <Link to={cta.to} className="btn-primary mt-4">
          {cta.label}
        </Link>
      )}
    </div>
  );
}
