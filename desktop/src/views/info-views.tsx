// The smaller views: Doctor, Leases and About.

import { openUrl } from "@tauri-apps/plugin-opener";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { refreshDoctor, refreshLeases, useSthin } from "@/store";

const RELEASES = "https://github.com/wktorbert/sthin/releases";

const statusTone: Record<string, string> = {
  ok: "text-success",
  warn: "text-warning",
  fail: "text-destructive",
  "n/a": "text-muted-foreground",
};

export function DoctorView() {
  const doctor = useSthin((s) => s.doctor);
  return (
    <div className="p-4">
      <header className="mb-3 flex items-center gap-3">
        <h2 className="text-lg font-semibold">Doctor</h2>
        {doctor && <span className={doctor.blocking ? "text-sm text-destructive" : "text-sm text-success"}>{doctor.blocking ? "A check blocks slim boot" : "Ready to slim boot"}</span>}
        <Button size="sm" variant="outline" className="ml-auto" onClick={() => void refreshDoctor()}>
          Run again
        </Button>
      </header>
      {!doctor ? (
        <p className="text-sm text-muted-foreground">Running checks…</p>
      ) : (
        <table className="w-full text-sm">
          <tbody>
            {doctor.checks.map((c) => (
              <tr key={c.name} className="border-b last:border-0">
                <td className={cn("w-14 py-1.5 font-mono text-xs", statusTone[c.status])}>{c.status}</td>
                <td className="w-56 py-1.5">{c.name}</td>
                <td className="py-1.5 text-muted-foreground select-text">{c.detail}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

export function LeasesView() {
  const leases = useSthin((s) => s.leases);
  const devices = useSthin((s) => s.devices);
  return (
    <div className="p-4">
      <header className="mb-3 flex items-center gap-3">
        <h2 className="text-lg font-semibold">Leases</h2>
        <span className="text-sm text-muted-foreground">Devices held by agents and scripts through `sthin lease` or MCP.</span>
        <Button size="sm" variant="outline" className="ml-auto" onClick={() => void refreshLeases()}>
          Refresh
        </Button>
      </header>
      {leases.length === 0 ? (
        <p className="text-sm text-muted-foreground">No active leases.</p>
      ) : (
        <table className="w-full text-sm">
          <thead className="text-left text-muted-foreground">
            <tr>
              <th className="py-1 font-normal">Device</th>
              <th className="py-1 font-normal">Owner</th>
              <th className="py-1 font-normal">Expires</th>
            </tr>
          </thead>
          <tbody>
            {leases.map((l) => (
              <tr key={l.id} className="border-t">
                <td className="py-1.5">{devices.find((d) => d.id === l.device)?.name ?? l.device}</td>
                <td className="py-1.5">{l.owner || "unnamed"}</td>
                <td className="py-1.5 tabular-nums">{new Date(l.expires_at).toLocaleString()}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

export function AboutView() {
  const appVersion = useSthin((s) => s.appVersion);
  const os = useSthin((s) => s.init?.os);
  const preview = os === "windows";
  return (
    <div className="max-w-xl space-y-3 p-4 text-sm">
      <h2 className="text-lg font-semibold">Sthin {appVersion}</h2>
      {preview && <p className="text-warning">Windows builds are a preview: Android only, not yet verified on real hardware.</p>}
      <p className="text-muted-foreground">Boots iOS simulators and Android emulators already slimmed, and restores them to stock. The desktop app runs the same code as the `sthin` command line, TUI and MCP server, through a bundled `sthin serve`.</p>
      <Button variant="outline" onClick={() => void openUrl(RELEASES)}>
        Check for updates
      </Button>
    </div>
  );
}
