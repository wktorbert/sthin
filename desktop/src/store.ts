// The Desktop's one store, fed by Serve: request results and push
// notifications. Components read from it; only the actions below talk to Serve.

import { getVersion } from "@tauri-apps/api/app";
import { create } from "zustand";
import { call, cancel, onNotification, onSidecarRestarted, onSidecarStatus, ServeCallError, sidecarStatus, start, type SidecarStatus } from "@/serve/client";
import type { BootParams, Check, Device, Initialize, Lease, Notification, PlatformStatus, Stage } from "@/serve/protocol";
import { LogBuffer, type LogFilter } from "@/lib/log-buffer";
import { checkSidecar } from "@/lib/sidecar-gate";
import { applyStage } from "@/lib/stage-progress";
import { notifyDone } from "@/notify";
import { Level } from "@/serve/protocol";

export type OpKind = "boot" | "restore";

/** A boot or restore in flight or just finished, per device. */
export interface Op {
  kind: OpKind;
  requestId: number;
  stages: Stage[];
  running: boolean;
  error?: string;
  /** Started from the Tray: report the outcome as an OS notification. */
  fromTray: boolean;
}

export type View = "devices" | "logs" | "doctor" | "leases" | "about";

interface LogView {
  deviceId: string;
  requestId: number | null;
  buffer: LogBuffer;
  /** Bumped when the buffer changes, so views re-render. */
  version: number;
  filter: LogFilter;
  follow: boolean;
  ended?: string;
}

interface State {
  phase: "starting" | "ready" | "refused";
  refusal?: string;
  sidecar: SidecarStatus;
  appVersion: string;
  init?: Initialize;
  platforms: PlatformStatus[];
  devices: Device[];
  selectedId?: string;
  ops: Record<string, Op>;
  doctor?: { checks: Check[]; blocking: boolean };
  leases: Lease[];
  view: View;
  log?: LogView;
  toast?: string;
}

export const useLean = create<State>(() => ({
  phase: "starting",
  sidecar: { state: "starting" },
  appVersion: "",
  platforms: [],
  devices: [],
  ops: {},
  leases: [],
  view: "devices",
}));

const set = useLean.setState;
const get = useLean.getState;
let subscriptionId: number | null = null;
// Bumped by every openLogs/closeLogs, so a slower, older openLogs can tell it
// was superseded and must not overwrite the view or leave a stream running.
let logSession = 0;

/** Runs once at launch: listeners first, then the handshake. */
export async function startApp(): Promise<void> {
  await onNotification(route);
  await onSidecarStatus((s) => set({ sidecar: s }));
  await onSidecarRestarted(() => void connect());
  set({ appVersion: await getVersion(), sidecar: await sidecarStatus() });
  await connect();
}

/** Handshake, version gate, then the data the window shows. Re-run after a restart. */
export async function connect(): Promise<void> {
  try {
    const init = await call("initialize", { client_name: "lean-desktop", client_version: get().appVersion });
    const gate = checkSidecar(get().appVersion, init);
    if (!gate.ok) {
      set({ phase: "refused", refusal: gate.reason, init });
      return;
    }
    set({ init, phase: "ready" });
    // After a restart the old process's log stream is gone and will never end.
    const log = get().log;
    if (log?.requestId != null) set({ log: { ...log, requestId: null, ended: "Lean restarted; reopen the log to follow it again" } });
    const s = start("devices_subscribe", {});
    subscriptionId = s.id;
    const listing = await s.result;
    set({ platforms: listing.platforms, devices: listing.devices });
    void refreshDoctor();
    void refreshLeases();
  } catch (e) {
    set({ toast: message(e) });
  }
}

function route(n: Notification): void {
  switch (n.method) {
    case "devices":
      if (n.params.id === subscriptionId) set({ platforms: n.params.platforms, devices: n.params.devices });
      return;
    case "progress": {
      const entry = Object.entries(get().ops).find(([, op]) => op.requestId === n.params.id);
      if (!entry) return;
      const [deviceId, op] = entry;
      set({ ops: { ...get().ops, [deviceId]: { ...op, stages: applyStage(op.stages, n.params.stage) } } });
      return;
    }
    case "log_lines": {
      const log = get().log;
      if (!log || log.requestId !== n.params.id) return;
      log.buffer.add(n.params.lines);
      set({ log: { ...log, version: log.version + 1 } });
      return;
    }
    case "subscription_end": {
      const log = get().log;
      if (log && log.requestId === n.params.id) set({ log: { ...log, requestId: null, ended: n.params.reason } });
    }
  }
}

export function select(id: string): void {
  set({ selectedId: id, view: "devices" });
}

export function setView(view: View): void {
  set({ view });
}

export function dismissToast(): void {
  set({ toast: undefined });
}

/** Boots or restores with live stages; the outcome stays on screen afterwards. */
async function runOp(kind: OpKind, deviceId: string, fromTray: boolean, params: BootParams): Promise<void> {
  if (get().ops[deviceId]?.running) return;
  const s = kind === "boot" ? start("boot", params) : start("restore", { id: deviceId });
  set({ ops: { ...get().ops, [deviceId]: { kind, requestId: s.id, stages: [], running: true, fromTray } } });
  let error: string | undefined;
  try {
    await s.result;
  } catch (e) {
    error = message(e);
  }
  const op = get().ops[deviceId];
  set({ ops: { ...get().ops, [deviceId]: { ...op, running: false, error } } });
  const name = get().devices.find((d) => d.id === deviceId)?.name ?? deviceId;
  if (fromTray) void notifyDone(kind, name, error);
}

export function bootDevice(id: string, opts: Omit<BootParams, "id"> = {}, fromTray = false): Promise<void> {
  return runOp("boot", id, fromTray, { id, ...opts });
}

export function restoreDevice(id: string, fromTray = false): Promise<void> {
  return runOp("restore", id, fromTray, { id });
}

export function cancelOp(deviceId: string): void {
  const op = get().ops[deviceId];
  if (op?.running) void cancel(op.requestId);
}

export async function shutdownDevice(id: string): Promise<void> {
  try {
    await call("shutdown", { id });
  } catch (e) {
    set({ toast: message(e) });
  }
}

export async function refreshDoctor(): Promise<void> {
  try {
    set({ doctor: await call("doctor", {}) });
  } catch (e) {
    set({ toast: message(e) });
  }
}

export async function refreshLeases(): Promise<void> {
  try {
    set({ leases: (await call("leases", {})).leases });
  } catch (e) {
    set({ toast: message(e) });
  }
}

/** Opens the log viewer on a device: a snapshot, then live lines if asked. */
export async function openLogs(deviceId: string): Promise<void> {
  await closeLogs();
  const session = ++logSession;
  const log: LogView = { deviceId, requestId: null, buffer: new LogBuffer(), version: 0, filter: { text: "", minLevel: Level.Verbose }, follow: true };
  set({ log, view: "logs" });
  try {
    const snap = await call("logs", { id: deviceId, lines: 500 });
    if (session !== logSession) return;
    log.buffer.add(snap.lines);
    const s = start("logs_follow", { id: deviceId });
    set({ log: { ...log, requestId: s.id, version: 1 } });
    await s.result;
    if (session !== logSession) void cancel(s.id);
  } catch (e) {
    if (session === logSession) set({ log: { ...log, ended: message(e), version: 1 } });
  }
}

export async function closeLogs(): Promise<void> {
  logSession++;
  const log = get().log;
  if (log?.requestId != null) await cancel(log.requestId);
  set({ log: undefined });
}

export function updateLog(patch: Partial<Pick<LogView, "filter" | "follow">>): void {
  const log = get().log;
  if (log) set({ log: { ...log, ...patch } });
}

export function clearLog(): void {
  const log = get().log;
  if (!log) return;
  log.buffer.clear();
  set({ log: { ...log, version: log.version + 1 } });
}

export function message(e: unknown): string {
  if (e instanceof ServeCallError) return e.error.message;
  return e instanceof Error ? e.message : String(e);
}
