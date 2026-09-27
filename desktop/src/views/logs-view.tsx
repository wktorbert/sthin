// The device log viewer: filter, level, follow, clear, copy. Lines arrive in
// batches from Serve into a 50,000-line buffer; only the newest matching lines
// are rendered so a chatty device never stalls the window.

import { useEffect, useMemo, useRef } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { matches } from "@/lib/log-buffer";
import { cn } from "@/lib/utils";
import { Level, type LogLine } from "@/serve/protocol";
import { clearLog, closeLogs, updateLog, useSthin } from "@/store";

const RENDERED = 2000;
const levelNames = ["verbose", "debug", "info", "warn", "error", "fatal"];
const levelTone = ["text-muted-foreground", "text-muted-foreground", "", "text-warning", "text-destructive", "text-pink"];

export function LogsView() {
  const log = useSthin((s) => s.log);
  const name = useSthin((s) => s.devices.find((d) => d.id === s.log?.deviceId)?.name);
  const bottom = useRef<HTMLDivElement>(null);

  const shown = useMemo(() => {
    if (!log) return { lines: [] as LogLine[], total: 0 };
    const all = log.buffer.lines().filter((l) => matches(l, log.filter));
    return { lines: all.slice(-RENDERED), total: all.length };
    // `version` changes whenever the buffer does.
  }, [log?.version, log?.filter, log]);

  useEffect(() => {
    if (log?.follow) bottom.current?.scrollIntoView({ block: "end" });
  }, [shown, log?.follow]);

  if (!log) return <p className="p-4 text-sm text-muted-foreground">No log open. Select a device and choose Open logs.</p>;

  const copy = () => void navigator.clipboard.writeText(shown.lines.map((l) => l.raw).join("\n"));

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex flex-wrap items-center gap-2 border-b p-2">
        <span className="px-1 text-sm font-medium">{name ?? log.deviceId}</span>
        <Input className="h-8 w-64" placeholder="Filter text" value={log.filter.text} onChange={(e) => updateLog({ filter: { ...log.filter, text: e.target.value } })} />
        <Select value={String(log.filter.minLevel)} onValueChange={(v) => updateLog({ filter: { ...log.filter, minLevel: Number(v) as Level } })}>
          <SelectTrigger size="sm" className="w-28" aria-label="Minimum level">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {levelNames.map((n, i) => (
              <SelectItem key={n} value={String(i)}>
                {n}+
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button size="sm" variant={log.follow ? "default" : "outline"} onClick={() => updateLog({ follow: !log.follow })}>
          Follow
        </Button>
        <Button size="sm" variant="outline" onClick={clearLog}>
          Clear
        </Button>
        <Button size="sm" variant="outline" onClick={copy}>
          Copy shown
        </Button>
        <span className="ml-auto text-xs text-muted-foreground tabular-nums">
          {shown.total > RENDERED ? `newest ${RENDERED} of ${shown.total}` : `${shown.total}`} lines
          {log.buffer.dropped > 0 && ` · ${log.buffer.dropped} older dropped`}
          {log.ended && ` · ${log.ended}`}
        </span>
        <Button size="sm" variant="ghost" onClick={() => void closeLogs()}>
          Close
        </Button>
      </div>
      <div className="min-h-0 flex-1 overflow-auto p-2 font-mono text-xs leading-5 select-text" onWheel={(e) => e.deltaY < 0 && log.follow && updateLog({ follow: false })}>
        {shown.lines.map((l, i) => (
          <div key={i} className={cn("whitespace-pre-wrap break-all", levelTone[l.level])}>
            <span className="text-muted-foreground">{l.time} </span>
            {l.process && <span className="text-info">{l.process} </span>}
            {l.message || l.raw}
          </div>
        ))}
        <div ref={bottom} />
      </div>
    </div>
  );
}
