import type { Device, Platform, PlatformStatus } from "@/serve/protocol";

// The Tray's menu as data, built from the device listing. tray.ts turns it into
// a native menu; keeping it pure makes the rules testable.

export interface TrayAction {
  /** `<verb>:<device id>`; see parseTrayAction. */
  id: string;
  text: string;
  enabled: boolean;
}

export type TrayEntry =
  | { type: "header"; text: string }
  | { type: "empty"; text: string }
  | { type: "device"; id: string; text: string; items: TrayAction[] }
  | { type: "separator" }
  | { type: "action"; id: "show" | "doctor" | "quit"; text: string };

export type TrayVerb = "boot" | "shutdown" | "restore" | "progress" | "show" | "doctor" | "quit";

const sections: Record<Platform, { title: string; empty: string }> = {
  ios: { title: "iOS Simulators", empty: "No simulators" },
  android: { title: "Android Emulators", empty: "No emulators" },
};

/**
 * One section per available platform listing its virtual devices. Physical
 * devices are never booted or modified, so they are not in the Tray. A device
 * with an operation in flight (`busy`) offers only its progress view.
 */
export function trayModel(platforms: PlatformStatus[], devices: Device[], busy: ReadonlySet<string>): TrayEntry[] {
  const out: TrayEntry[] = [];
  for (const p of platforms) {
    if (!p.available) continue;
    const s = sections[p.platform];
    out.push({ type: "header", text: s.title });
    const ds = devices.filter((d) => d.platform === p.platform && d.kind !== "physical");
    if (ds.length === 0) out.push({ type: "empty", text: s.empty });
    for (const d of ds) out.push({ type: "device", id: d.id, text: label(d), items: actions(d, busy.has(d.id)) });
  }
  out.push(
    { type: "separator" },
    { type: "action", id: "show", text: "Show Lean" },
    { type: "action", id: "doctor", text: "Doctor" },
    { type: "action", id: "quit", text: "Quit Lean" },
  );
  return out;
}

function label(d: Device): string {
  return `${d.state === "booted" ? "●" : "○"} ${d.name} · ${d.os_version}`;
}

function actions(d: Device, busy: boolean): TrayAction[] {
  if (busy) return [{ id: `progress:${d.id}`, text: "Working… show progress", enabled: true }];
  const booted = d.state === "booted";
  return [
    { id: `boot:${d.id}`, text: "Boot slim", enabled: !booted },
    { id: `shutdown:${d.id}`, text: "Shut down", enabled: booted },
    { id: `restore:${d.id}`, text: "Restore to stock", enabled: d.slim !== "stock" },
  ];
}

export function parseTrayAction(id: string): { verb: TrayVerb; id: string } {
  const i = id.indexOf(":");
  if (i < 0) return { verb: id as TrayVerb, id: "" };
  return { verb: id.slice(0, i) as TrayVerb, id: id.slice(i + 1) };
}
