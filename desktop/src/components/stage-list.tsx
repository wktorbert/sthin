// A boot or restore's stages as they stream in, with the failed command shown.

import { cn } from "@/lib/utils";
import type { Stage } from "@/serve/protocol";

const marks: Record<Stage["status"], { mark: string; tone: string }> = {
  running: { mark: "…", tone: "text-warning" },
  ok: { mark: "✓", tone: "text-success" },
  fail: { mark: "✗", tone: "text-destructive" },
  warn: { mark: "!", tone: "text-warning" },
  skip: { mark: "–", tone: "text-muted-foreground" },
};

export function StageList({ stages }: { stages: Stage[] }) {
  if (stages.length === 0) return <p className="text-sm text-muted-foreground">Starting…</p>;
  return (
    <ol className="space-y-1 font-mono text-xs">
      {stages.map((s, i) => {
        const m = marks[s.status];
        return (
          <li key={`${s.name}-${i}`} className="flex gap-2">
            <span className={cn("w-3 shrink-0 text-center", m.tone, s.status === "running" && "animate-pulse")}>{m.mark}</span>
            <span className="w-36 shrink-0 truncate">{s.name}</span>
            <span className="min-w-0 flex-1 break-words text-muted-foreground select-text">
              {s.measurement ? `${s.measurement.footprint_mb} MB, ${s.measurement.process_count} processes` : s.detail}
              {s.command && <code className="mt-0.5 block text-destructive">{s.command}</code>}
            </span>
          </li>
        );
      })}
    </ol>
  );
}
