import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { Plus, Camera as CameraIcon, Edit2, Trash2, Wifi, X, Loader2, CheckCircle2, AlertTriangle, Play } from "lucide-react";
import {
  listCameras,
  createCamera,
  updateCamera,
  deleteCamera,
  probeCamera,
} from "../lib/api";
import type { Camera, ProbeResult } from "../lib/types";
import { PasswordInput } from "../components/PasswordInput";
import { useEscapeKey } from "../lib/hooks";

const emptyCamera: Partial<Camera> = {
  name: "",
  vendor: "hikvision",
  host: "",
  port: 554,
  username: "admin",
  password: "",
  channel: 1,
  useSubStream: false,
  rawRtspUrl: "",
};

export function Cameras() {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const camerasQ = useQuery({ queryKey: ["cameras"], queryFn: listCameras });
  const [editing, setEditing] = useState<Partial<Camera> | null>(null);

  const delMut = useMutation({
    mutationFn: deleteCamera,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["cameras"] }),
  });

  const cameras = camerasQ.data ?? [];

  return (
    <div className="p-6 space-y-6">
      <header className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Kameralar</h1>
          <p className="text-sm text-gray-400 mt-1">
            IP kameralarni qo'shing va ulanishni tekshiring
          </p>
        </div>
        <button onClick={() => setEditing(emptyCamera)} className="btn-primary">
          <Plus className="w-4 h-4" />
          Yangi kamera
        </button>
      </header>

      {camerasQ.isLoading ? (
        <div className="card p-12 flex items-center justify-center text-gray-500">
          <Loader2 className="w-5 h-5 animate-spin mr-2" />
          Yuklanmoqda...
        </div>
      ) : cameras.length === 0 ? (
        <div className="card p-12 flex flex-col items-center justify-center text-center">
          <CameraIcon className="w-12 h-12 text-gray-500 mb-3" />
          <p className="font-medium">Kameralar yo'q</p>
          <p className="text-sm text-gray-400 mt-1">
            Hikvision yoki boshqa IP kamerangizni qo'shing
          </p>
          <button onClick={() => setEditing(emptyCamera)} className="btn-primary mt-4">
            <Plus className="w-4 h-4" />
            Birinchi kamerani qo'shish
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {cameras.map((c) => (
            <CameraCard
              key={c.id}
              camera={c}
              onPreview={() => navigate(`/cameras/${c.id}/preview`)}
              onEdit={() => setEditing(c)}
              onDelete={() => {
                if (confirm(`"${c.name}" o'chirilsinmi?`)) delMut.mutate(c.id);
              }}
            />
          ))}
        </div>
      )}

      {editing && (
        <CameraFormModal
          camera={editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            qc.invalidateQueries({ queryKey: ["cameras"] });
          }}
        />
      )}
    </div>
  );
}

// ============================== Card ==============================

function CameraCard({
  camera,
  onPreview,
  onEdit,
  onDelete,
}: {
  camera: Camera;
  onPreview: () => void;
  onEdit: () => void;
  onDelete: () => void;
}) {
  return (
    <div className="card p-5 hover:border-white/10 transition-colors">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <h3 className="font-semibold truncate">{camera.name}</h3>
          <p className="text-xs text-gray-500 mt-0.5 truncate">
            {camera.vendor} • {camera.host}:{camera.port}
          </p>
        </div>
        <div className="flex gap-1">
          <button onClick={onEdit} className="btn-ghost p-1.5" title="Tahrirlash">
            <Edit2 className="w-4 h-4" />
          </button>
          <button onClick={onDelete} className="btn-ghost p-1.5 hover:text-danger" title="O'chirish">
            <Trash2 className="w-4 h-4" />
          </button>
        </div>
      </div>

      <div className="mt-4 space-y-1 text-xs text-gray-400">
        <div className="flex justify-between">
          <span>Kanal</span>
          <span className="text-gray-200">{camera.channel}</span>
        </div>
        <div className="flex justify-between">
          <span>Sub-stream</span>
          <span className="text-gray-200">{camera.useSubStream ? "Ha" : "Yo'q"}</span>
        </div>
      </div>

      <button
        onClick={onPreview}
        className="btn-secondary w-full mt-4 text-sm"
        title="Kamerani jonli ko'rish"
      >
        <Play className="w-4 h-4" />
        Jonli ko'rish
      </button>
    </div>
  );
}

// ============================== Form Modal ==============================

function CameraFormModal({
  camera,
  onClose,
  onSaved,
}: {
  camera: Partial<Camera>;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [form, setForm] = useState<Partial<Camera>>(camera);
  const [probing, setProbing] = useState(false);
  const [probeResult, setProbeResult] = useState<ProbeResult | null>(null);
  const isNew = !form.id;

  useEscapeKey(onClose);

  const saveMut = useMutation({
    mutationFn: async () => {
      if (isNew) return createCamera(form);
      return updateCamera(form as Camera);
    },
    onSuccess: onSaved,
  });

  const handleProbe = async () => {
    setProbing(true);
    setProbeResult(null);
    try {
      const r = await probeCamera(form);
      setProbeResult(r);
    } catch (e) {
      setProbeResult({ ok: false, codec: "", width: 0, height: 0, fps: "", hasAudio: false, audioCodec: "", error: String(e) });
    } finally {
      setProbing(false);
    }
  };

  const update = (k: keyof Camera) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
    const v = e.target.type === "number" ? Number(e.target.value)
      : e.target.type === "checkbox" ? (e.target as HTMLInputElement).checked
      : e.target.value;
    setForm({ ...form, [k]: v });
  };

  return (
    <div className="fixed inset-0 bg-black/85 backdrop-blur flex items-center justify-center p-4 z-50 animate-in fade-in duration-150" onClick={onClose}>
      <div className="bg-bg-card rounded-xl border border-white/10 shadow-2xl w-full max-w-2xl max-h-[90vh] overflow-auto" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between p-5 border-b border-white/5">
          <h2 className="text-lg font-semibold">
            {isNew ? "Yangi kamera" : "Kamerani tahrirlash"}
          </h2>
          <button onClick={onClose} className="btn-ghost p-1">
            <X className="w-5 h-5" />
          </button>
        </div>

        <div className="p-5 space-y-4">
          <div>
            <label className="label">Nom</label>
            <input className="input" placeholder="Masalan: Eshik oldi" value={form.name ?? ""} onChange={update("name")} />
          </div>

          <div>
            <label className="label">Vendor</label>
            <select className="input" value={form.vendor} onChange={update("vendor")}>
              <option value="hikvision">Hikvision</option>
              <option value="dahua">Dahua</option>
              <option value="generic">Boshqa (qo'lda URL)</option>
            </select>
          </div>

          {form.vendor === "generic" ? (
            <div>
              <label className="label">RTSP URL</label>
              <input
                className="input font-mono text-xs"
                placeholder="rtsp://user:pass@192.168.1.64:554/..."
                value={form.rawRtspUrl ?? ""}
                onChange={update("rawRtspUrl")}
              />
            </div>
          ) : (
            <>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="label">Host / IP</label>
                  <input className="input" placeholder="192.168.1.64" value={form.host ?? ""} onChange={update("host")} />
                </div>
                <div>
                  <label className="label">Port</label>
                  <input className="input" type="number" value={form.port ?? 554} onChange={update("port")} />
                </div>
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="label">Username</label>
                  <input className="input" value={form.username ?? ""} onChange={update("username")} />
                </div>
                <div>
                  <label className="label">Password</label>
                  <PasswordInput
                    value={form.password ?? ""}
                    onChange={update("password")}
                  />
                </div>
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="label">Kanal</label>
                  <input className="input" type="number" min={1} value={form.channel ?? 1} onChange={update("channel")} />
                </div>
                <div>
                  <label className="label">Sub-stream</label>
                  <div className="flex items-center gap-2 mt-2.5">
                    <input
                      type="checkbox"
                      id="useSubStream"
                      checked={form.useSubStream ?? false}
                      onChange={update("useSubStream")}
                      className="w-4 h-4 rounded bg-bg-subtle border-white/10"
                    />
                    <label htmlFor="useSubStream" className="text-sm">
                      Ikkilamchi oqim (past sifat, kam trafik)
                    </label>
                  </div>
                </div>
              </div>
            </>
          )}

          {/* Probe natijasi */}
          {probeResult && (
            <div className={`rounded-lg p-3 border ${probeResult.ok ? "bg-success/10 border-success/30" : "bg-danger/10 border-danger/30"}`}>
              <div className="flex items-start gap-2">
                {probeResult.ok ? (
                  <CheckCircle2 className="w-5 h-5 text-success flex-shrink-0 mt-0.5" />
                ) : (
                  <AlertTriangle className="w-5 h-5 text-danger flex-shrink-0 mt-0.5" />
                )}
                <div className="text-sm flex-1 min-w-0">
                  {probeResult.ok ? (
                    <>
                      <p className="font-medium text-success">Ulanish muvaffaqiyatli</p>
                      <p className="text-gray-300 text-xs mt-1">
                        {probeResult.width}×{probeResult.height} {probeResult.codec} @ {probeResult.fps}
                        {probeResult.hasAudio && ` • audio: ${probeResult.audioCodec}`}
                      </p>
                    </>
                  ) : (
                    <>
                      <p className="font-medium text-danger">Ulanib bo'lmadi</p>
                      <p className="text-gray-300 text-xs mt-1 break-words">{probeResult.error}</p>
                    </>
                  )}
                </div>
              </div>
            </div>
          )}

          {saveMut.error && (
            <div className="text-sm text-danger">{String(saveMut.error)}</div>
          )}
        </div>

        <div className="flex items-center justify-between gap-3 p-5 border-t border-white/5">
          <button onClick={handleProbe} disabled={probing} className="btn-secondary">
            {probing ? <Loader2 className="w-4 h-4 animate-spin" /> : <Wifi className="w-4 h-4" />}
            Ulanishni tekshirish
          </button>
          <div className="flex gap-2">
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
    </div>
  );
}
