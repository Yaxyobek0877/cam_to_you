import { useEffect, useRef, useState } from "react";
import { Trash2, Pause, Play, ArrowDown, ScrollText, AlertCircle, Info, AlertTriangle } from "lucide-react";
import {
  getEntries,
  subscribe,
  clearEntries,
  type LogEntry,
  type LogSource,
  type LogLevel,
} from "../lib/eventStore";
import { cn } from "../lib/utils";

const sourceLabels: Record<LogSource, { label: string; color: string }> = {
  stream:  { label: "stream",  color: "text-accent" },
  preview: { label: "preview", color: "text-purple-400" },
  ffmpeg:  { label: "ffmpeg",  color: "text-orange-400" },
  system:  { label: "system",  color: "text-gray-400" },
};

const levelIcons: Record<LogLevel, { icon: React.ElementType; color: string }> = {
  info:    { icon: Info,           color: "text-gray-400" },
  warning: { icon: AlertTriangle,  color: "text-warning"  },
  error:   { icon: AlertCircle,    color: "text-danger"   },
};

export function Logs() {
  const [entries, setEntries] = useState<LogEntry[]>(getEntries());
  const [paused, setPaused] = useState(false);
  const [autoScroll, setAutoScroll] = useState(true);
  const [filterSource, setFilterSource] = useState<LogSource | "all">("all");
  const [filterLevel, setFilterLevel] = useState<LogLevel | "all">("all");
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (paused) return;
    const unsub = subscribe((newEntries) => {
      setEntries([...newEntries]);
    });
    return unsub;
  }, [paused]);

  // Auto-scroll
  useEffect(() => {
    if (autoScroll && containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [entries, autoScroll]);

  const filtered = entries.filter((e) => {
    if (filterSource !== "all" && e.source !== filterSource) return false;
    if (filterLevel !== "all" && e.level !== filterLevel) return false;
    return true;
  });

  return (
    <div className="p-6 h-full flex flex-col gap-4 max-h-screen overflow-hidden">
      {/* Header */}
      <header className="flex items-center justify-between flex-shrink-0">
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <ScrollText className="w-6 h-6" />
            Loglar
          </h1>
          <p className="text-sm text-gray-400 mt-1">
            Stream, preview va tizim hodisalari real-time
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setPaused(!paused)}
            className={paused ? "btn-primary" : "btn-secondary"}
            title={paused ? "Davom ettirish" : "To'xtatib turish"}
          >
            {paused ? <Play className="w-4 h-4" /> : <Pause className="w-4 h-4" />}
            {paused ? "Davom ettirish" : "Pauza"}
          </button>
          <button
            onClick={() => {
              if (confirm("Barcha loglar tozalansinmi?")) clearEntries();
            }}
            className="btn-secondary"
          >
            <Trash2 className="w-4 h-4" />
            Tozalash
          </button>
        </div>
      </header>

      {/* Filterlar */}
      <div className="flex items-center gap-3 flex-shrink-0 card p-3">
        <span className="text-xs text-gray-500">Filter:</span>
        <select
          className="input py-1 text-xs flex-1 max-w-[160px]"
          value={filterSource}
          onChange={(e) => setFilterSource(e.target.value as any)}
        >
          <option value="all">Barcha manbalar</option>
          <option value="stream">Stream</option>
          <option value="preview">Preview</option>
          <option value="ffmpeg">FFmpeg</option>
          <option value="system">System</option>
        </select>
        <select
          className="input py-1 text-xs flex-1 max-w-[160px]"
          value={filterLevel}
          onChange={(e) => setFilterLevel(e.target.value as any)}
        >
          <option value="all">Barcha darajalar</option>
          <option value="info">Info</option>
          <option value="warning">Warning</option>
          <option value="error">Error</option>
        </select>
        <label className="flex items-center gap-1.5 text-xs text-gray-400 ml-auto cursor-pointer">
          <input
            type="checkbox"
            checked={autoScroll}
            onChange={(e) => setAutoScroll(e.target.checked)}
            className="w-3.5 h-3.5"
          />
          <ArrowDown className="w-3.5 h-3.5" />
          Avto-skroll
        </label>
        <span className="text-xs text-gray-500 tabular-nums">
          {filtered.length} / {entries.length}
        </span>
      </div>

      {/* Log oqimi */}
      <div
        ref={containerRef}
        className="card flex-1 overflow-y-auto font-mono text-xs leading-relaxed p-2 min-h-0"
      >
        {filtered.length === 0 ? (
          <div className="flex items-center justify-center h-full text-gray-500">
            {entries.length === 0 ? "Hali loglar yo'q — biror amal qiling" : "Filter shartlariga mos log yo'q"}
          </div>
        ) : (
          filtered.map((e) => <LogRow key={e.id} entry={e} />)
        )}
      </div>
    </div>
  );
}

function LogRow({ entry }: { entry: LogEntry }) {
  const source = sourceLabels[entry.source];
  const level = levelIcons[entry.level];
  const Icon = level.icon;

  const time = entry.time.toLocaleTimeString("uz-UZ", { hour12: false }) +
    "." + String(entry.time.getMilliseconds()).padStart(3, "0");

  return (
    <div className="flex items-start gap-2 py-1 px-2 hover:bg-bg-subtle/40 rounded">
      <span className="text-gray-600 tabular-nums flex-shrink-0">{time}</span>
      <Icon className={cn("w-3.5 h-3.5 flex-shrink-0 mt-0.5", level.color)} />
      <span className={cn("flex-shrink-0", source.color)}>
        [{source.label}{entry.sourceId !== undefined ? `#${entry.sourceId}` : ""}]
      </span>
      <span className={cn("flex-1 break-words", entry.level === "error" ? "text-danger" : "text-gray-200")}>
        {entry.message}
      </span>
    </div>
  );
}
