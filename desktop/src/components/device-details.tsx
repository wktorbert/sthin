// The right column: the selected device, its actions and the live progress of
// its current or last boot/restore.

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { CategoryPicker } from "@/components/category-picker";
import { SlimBadge } from "@/components/device-panels";
import { StageList } from "@/components/stage-list";
import type { Device } from "@/serve/protocol";
import { bootDevice, cancelOp, openLogs, restoreDevice, shutdownDevice, useSthin } from "@/store";

export function DeviceDetails({ device }: { device?: Device }) {
  const op = useSthin((s) => (device ? s.ops[device.id] : undefined));
  const blocking = useSthin((s) => s.doctor?.blocking ?? false);
  const [picking, setPicking] = useState(false);
  const [confirmRestore, setConfirmRestore] = useState(false);

  if (!device) return <p className="p-4 text-sm text-muted-foreground">Select a device.</p>;
  const physical = device.kind === "physical";
  const booted = device.state === "booted";
  const busy = op?.running ?? false;
  const cannotBoot = blocking ? "Doctor found a problem that blocks slim boot. Open Doctor for details." : undefined;

  return (
    <div className="flex h-full flex-col gap-4 p-4">
      <div>
        <h2 className="flex items-center gap-2 text-lg font-semibold">
          {device.name} <SlimBadge d={device} />
        </h2>
        <dl className="mt-2 grid grid-cols-[7rem_1fr] gap-x-3 gap-y-1 text-sm">
          <Row k="ID" v={device.id} mono />
          <Row k="Platform" v={`${device.platform} · ${device.kind}${device.model ? ` · ${device.model}` : ""}`} />
          <Row k="OS" v={device.os_version} />
          <Row k="State" v={device.state} />
          <Row k="Memory" v={device.footprint_mb != null ? `${device.footprint_mb} MB` : "—"} />
          {device.lease && <Row k="Lease" v={`${device.lease.owner || "unnamed"} until ${new Date(device.lease.expires_at).toLocaleTimeString()}`} />}
        </dl>
        {(device.warnings?.length ?? 0) > 0 && (
          <ul className="mt-2 space-y-1 text-sm text-warning select-text">
            {device.warnings!.map((w) => (
              <li key={w}>⚠ {w}</li>
            ))}
          </ul>
        )}
      </div>

      {physical ? (
        <p className="text-sm text-muted-foreground">Physical devices are listed and used, never slimmed, booted or modified.</p>
      ) : (
        <div className="flex flex-wrap gap-2">
          <Button disabled={busy || booted || !!cannotBoot} title={cannotBoot} onClick={() => bootDevice(device.id)}>
            Boot slim
          </Button>
          <Button variant="secondary" disabled={busy || booted || !!cannotBoot} title={cannotBoot} onClick={() => setPicking(true)}>
            Categories…
          </Button>
          <Button variant="secondary" disabled={busy || booted} onClick={() => bootDevice(device.id, { stock: true })}>
            Boot stock
          </Button>
          <Button variant="secondary" disabled={busy || booted || !!cannotBoot} onClick={() => bootDevice(device.id, { headless: true })}>
            Boot headless
          </Button>
          <Button variant="outline" disabled={busy || !booted} onClick={() => shutdownDevice(device.id)}>
            Shut down
          </Button>
          {confirmRestore ? (
            <span className="flex items-center gap-2 text-sm">
              Restore to stock?
              <Button size="sm" variant="destructive" onClick={() => (setConfirmRestore(false), restoreDevice(device.id))}>
                Yes
              </Button>
              <Button size="sm" variant="ghost" onClick={() => setConfirmRestore(false)}>
                No
              </Button>
            </span>
          ) : (
            <Button variant="outline" disabled={busy || device.slim === "stock"} onClick={() => setConfirmRestore(true)}>
              Restore…
            </Button>
          )}
        </div>
      )}
      <div>
        <Button variant="ghost" size="sm" onClick={() => openLogs(device.id)}>
          Open logs
        </Button>
      </div>

      {op && (
        <section className="min-h-0 flex-1 overflow-auto rounded-lg border bg-card p-3">
          <header className="mb-2 flex items-center justify-between text-sm">
            <span className="font-medium">
              {op.kind === "boot" ? "Boot" : "Restore"} {op.running ? "in progress" : op.error ? "failed" : "finished"}
            </span>
            {op.running && (
              <Button size="sm" variant="ghost" onClick={() => cancelOp(device.id)}>
                Cancel
              </Button>
            )}
          </header>
          <StageList stages={op.stages} />
          {op.error && <p className="mt-2 text-sm text-destructive select-text">{op.error}</p>}
        </section>
      )}
      <CategoryPicker device={device} open={picking} onOpenChange={setPicking} />
    </div>
  );
}

function Row({ k, v, mono }: { k: string; v: string; mono?: boolean }) {
  return (
    <>
      <dt className="text-muted-foreground">{k}</dt>
      <dd className={mono ? "truncate font-mono text-xs leading-5 select-text" : "select-text"}>{v}</dd>
    </>
  );
}
