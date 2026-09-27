// The left column: one panel per platform plus physical devices, like the TUI.

import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import type { Panel } from "@/lib/device-panels";
import type { Device } from "@/serve/protocol";
import { select, useSthin } from "@/store";

export function DevicePanels({ panels }: { panels: Panel[] }) {
  return (
    <div className="flex flex-col gap-3">
      {panels.map((p) => (
        <section key={p.key} className="rounded-lg border bg-card">
          <h2 className="border-b px-3 py-2 text-xs font-semibold tracking-wide text-primary uppercase">
            {p.title} <span className="text-muted-foreground">({p.devices.length})</span>
          </h2>
          {p.devices.length === 0 ? (
            <p className="px-3 py-3 text-sm text-muted-foreground">None found</p>
          ) : (
            <ul role="listbox" aria-label={p.title}>
              {p.devices.map((d) => (
                <DeviceRow key={d.id} d={d} />
              ))}
            </ul>
          )}
        </section>
      ))}
    </div>
  );
}

function DeviceRow({ d }: { d: Device }) {
  const selected = useSthin((s) => s.selectedId === d.id);
  const running = useSthin((s) => s.ops[d.id]?.running ?? false);
  const booted = d.state === "booted";
  return (
    <li
      role="option"
      aria-selected={selected}
      tabIndex={0}
      onClick={() => select(d.id)}
      onKeyDown={(e) => e.key === "Enter" && select(d.id)}
      className={cn(
        "flex cursor-default items-center gap-2 px-3 py-1.5 text-sm outline-none focus-visible:bg-accent",
        selected && "bg-accent",
      )}
    >
      <span className={cn("size-2 shrink-0 rounded-full", booted ? "bg-success" : "bg-muted-foreground/40", running && "animate-pulse bg-warning")} />
      <span className="min-w-0 flex-1 truncate">{d.name}</span>
      <span className="text-xs text-muted-foreground tabular-nums">{d.os_version}</span>
      {d.lease && <Badge variant="outline" className="text-info">leased</Badge>}
      <SlimBadge d={d} />
      <span className="w-16 text-right text-xs text-muted-foreground tabular-nums">{d.footprint_mb != null ? `${d.footprint_mb} MB` : ""}</span>
      {(d.warnings?.length ?? 0) > 0 && <span className="text-warning" title={d.warnings!.join("\n")}>!</span>}
    </li>
  );
}

export function SlimBadge({ d }: { d: Device }) {
  if (d.slim === "n/a") return null;
  const tone = d.slim === "slim" ? "text-success" : d.slim === "partial" ? "text-warning" : "text-muted-foreground";
  return (
    <Badge variant="outline" className={cn("w-14 justify-center", tone)}>
      {d.slim}
    </Badge>
  );
}
