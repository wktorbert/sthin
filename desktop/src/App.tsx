// The window: header with navigation and status, then the current view.

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { DeviceDetails } from "@/components/device-details";
import { DevicePanels } from "@/components/device-panels";
import { WirelessADB } from "@/components/wireless-adb";
import { groupPanels } from "@/lib/device-panels";
import { cn } from "@/lib/utils";
import { retrySidecar } from "@/serve/client";
import { dismissToast, setView, useSthin, type View } from "@/store";
import { LogsView } from "@/views/logs-view";
import { AboutView, DoctorView, LeasesView } from "@/views/info-views";

const tabs: { view: View; label: string }[] = [
  { view: "devices", label: "Devices" },
  { view: "logs", label: "Logs" },
  { view: "doctor", label: "Doctor" },
  { view: "leases", label: "Leases" },
  { view: "about", label: "About" },
];

export function App() {
  const phase = useSthin((s) => s.phase);
  const sidecar = useSthin((s) => s.sidecar);
  const refusal = useSthin((s) => s.refusal);

  if (sidecar.state === "down") {
    return (
      <Blocked title="Sthin's engine stopped" body={sidecar.reason}>
        <pre className="max-h-64 overflow-auto rounded-md bg-card p-3 text-left text-xs select-text">{sidecar.stderr.join("\n")}</pre>
        <Button onClick={() => void retrySidecar()}>Retry</Button>
      </Blocked>
    );
  }
  if (phase === "refused") return <Blocked title="This copy of Sthin is broken" body={refusal ?? ""} />;
  if (phase === "starting") return <Blocked title="Starting Sthin…" body="" />;
  return <Main />;
}

function Main() {
  const view = useSthin((s) => s.view);
  const doctor = useSthin((s) => s.doctor);
  const toast = useSthin((s) => s.toast);
  const [adb, setAdb] = useState(false);
  return (
    <div className="flex h-full flex-col">
      <header className="flex items-center gap-1 border-b px-3 py-2" data-tauri-drag-region>
        <span className="mr-3 font-semibold text-primary">Sthin</span>
        {tabs.map((t) => (
          <Button key={t.view} size="sm" variant={view === t.view ? "secondary" : "ghost"} onClick={() => setView(t.view)}>
            {t.label}
            {t.view === "doctor" && doctor && <span className={cn("ml-1 size-2 rounded-full", doctor.blocking ? "bg-destructive" : "bg-success")} aria-label={doctor.blocking ? "problem" : "ok"} />}
          </Button>
        ))}
        <Button size="sm" variant="outline" className="ml-auto" onClick={() => setAdb(true)}>
          Wireless ADB
        </Button>
      </header>
      <Notices />
      <main className="min-h-0 flex-1">
        {view === "devices" && <DevicesView />}
        {view === "logs" && <LogsView />}
        {view === "doctor" && <DoctorView />}
        {view === "leases" && <LeasesView />}
        {view === "about" && <AboutView />}
      </main>
      {toast && (
        <div role="alert" className="flex items-center gap-3 border-t bg-card px-3 py-2 text-sm text-destructive">
          <span className="flex-1 select-text">{toast}</span>
          <Button size="sm" variant="ghost" onClick={dismissToast}>
            Dismiss
          </Button>
        </div>
      )}
      <WirelessADB open={adb} onOpenChange={setAdb} />
    </div>
  );
}

function Notices() {
  const platforms = useSthin((s) => s.platforms);
  const { notices } = groupPanels(platforms, []);
  if (notices.length === 0) return null;
  return (
    <div className="border-b bg-muted px-3 py-1.5 text-xs text-muted-foreground">
      {notices.map((n) => (
        <p key={n}>{n}</p>
      ))}
    </div>
  );
}

function DevicesView() {
  const platforms = useSthin((s) => s.platforms);
  const devices = useSthin((s) => s.devices);
  const selectedId = useSthin((s) => s.selectedId);
  const { panels } = groupPanels(platforms, devices);
  return (
    <div className="grid h-full min-h-0 grid-cols-[minmax(22rem,1.1fr)_1fr]">
      <div className="min-h-0 overflow-auto border-r p-3">
        <DevicePanels panels={panels} />
      </div>
      <div className="min-h-0 overflow-auto">
        <DeviceDetails device={devices.find((d) => d.id === selectedId)} />
      </div>
    </div>
  );
}

function Blocked({ title, body, children }: { title: string; body: string; children?: React.ReactNode }) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-4 p-8 text-center">
      <h1 className="text-xl font-semibold">{title}</h1>
      {body && <p className="max-w-lg text-sm text-muted-foreground select-text">{body}</p>}
      {children}
    </div>
  );
}
