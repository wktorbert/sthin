// Types for the `sthin serve` protocol. The contract is docs/serve-protocol.md;
// the Go source of truth is internal/serve. `Methods` is the one method list the
// typed client (client.ts) is derived from: adding a method means adding one
// entry here and nothing in Rust.

export type Platform = "ios" | "android";
export type Kind = "simulator" | "emulator" | "physical";
export type PowerState = "shutdown" | "booting" | "booted";
export type SlimState = "stock" | "slim" | "partial" | "unknown" | "n/a";
export type StageStatus = "running" | "ok" | "fail" | "warn" | "skip";

export interface Lease {
  id: string;
  device: string;
  platform: string;
  owner: string;
  created_at: string;
  expires_at: string;
}

export interface Device {
  id: string;
  name: string;
  platform: Platform;
  kind: Kind;
  model?: string;
  os_version: string;
  state: PowerState;
  slim: SlimState;
  footprint_mb: number | null;
  warnings: string[] | null;
  lease?: Lease;
}

export interface PlatformStatus {
  platform: Platform;
  available: boolean;
  reason?: string;
  error?: string;
}

export interface Measurement {
  id: string;
  platform: Platform;
  footprint_mb: number;
  process_count: number;
  dirty_mb?: number;
  guest_total_mb?: number;
  guest_used_mb?: number;
}

export interface Stage {
  name: string;
  status: StageStatus;
  detail?: string;
  command?: string;
  measurement?: Measurement;
}

/** Log level as the Go side encodes it: 0 verbose … 5 fatal. */
export enum Level {
  Verbose = 0,
  Debug = 1,
  Info = 2,
  Warn = 3,
  Error = 4,
  Fatal = 5,
}

export interface LogLine {
  time: string;
  level: Level;
  process: string;
  message: string;
  raw: string;
}

export interface Check {
  name: string;
  status: "ok" | "warn" | "fail" | "n/a";
  detail: string;
}

export interface Category {
  id: string;
  name: string;
  default: boolean;
  items: string[];
  approx_mb?: number;
}

export interface Profile {
  platform: Platform;
  version: number;
  validated_against: Record<string, string>;
  never_disable: string[];
  categories: Category[];
  aggressive: string[] | null;
}

export interface Listing {
  schema_version: number;
  platforms: PlatformStatus[];
  devices: Device[];
}

export interface BootParams {
  id: string;
  stock?: boolean;
  /** Omit to use the device's saved choice. */
  except?: string[];
  remember?: boolean;
  ram_mb?: number;
  headless?: boolean;
}

export interface Initialize {
  schema_version: number;
  sthin_version: string;
  os: string;
  platforms: PlatformStatus[];
}

type Id = { id: string };

/** Every Serve method: params and result. */
export interface Methods {
  initialize: { params: { client_name?: string; client_version?: string }; result: Initialize };
  devices_list: { params: { platform?: Platform }; result: Listing };
  devices_subscribe: { params: { platform?: Platform }; result: Listing };
  boot: { params: BootParams; result: { device: Device; stages: Stage[]; footprint_mb?: number } };
  restore: { params: Id; result: { device: string; stages: Stage[] } };
  shutdown: { params: Id; result: { device: string } };
  measure: { params: Id; result: Measurement };
  doctor: { params: Record<string, never>; result: { checks: Check[]; blocking: boolean } };
  profile: { params: { platform: Platform }; result: Profile };
  prefs_get: { params: Id; result: { id: string; except: string[]; saved: boolean } };
  leases: { params: Record<string, never>; result: { leases: Lease[] } };
  adb_pair: { params: { addr: string; code: string }; result: { message: string } };
  adb_connect: { params: { addr: string }; result: { message: string } };
  adb_disconnect: { params: { addr: string }; result: { message: string } };
  logs: { params: { id: string; lines?: number }; result: { device: string; lines: LogLine[] } };
  logs_follow: { params: Id; result: { device: string; following: true } };
  cancel: { params: { id: number }; result: { cancelled: true } };
}

export type Method = keyof Methods;

/** Notifications from Serve; `id` is the request they belong to. */
export type Notification =
  | { method: "progress"; params: { id: number; stage: Stage } }
  | { method: "devices"; params: { id: number; platforms: PlatformStatus[]; devices: Device[] } }
  | { method: "log_lines"; params: { id: number; lines: LogLine[] } }
  | { method: "subscription_end"; params: { id: number; reason: string } };

/** Error object from Serve, or from the Rust side when the sidecar is gone. */
export interface ServeError {
  code: number;
  message: string;
  data?: { stage?: string; command?: string; stages?: Stage[] };
}

/** Codes Sthin defines on top of JSON-RPC's. */
export const Codes = {
  Cancelled: -32800,
  /** Rust: the sidecar exited while the request was in flight. */
  ServeExited: -32001,
  /** Rust: the sidecar is not running (crash loop, or not started yet). */
  ServeDown: -32002,
  Failure: 1,
  Usage: 2,
  NoDevice: 3,
} as const;
