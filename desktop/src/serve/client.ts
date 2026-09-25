// The typed Serve client. Every call goes through Rust's one generic command,
// `serve_call`, which writes a JSON-RPC line to the sidecar and waits for the
// matching response. The id is chosen here so progress notifications and
// `cancel` can be matched to the call that started them.

import { invoke } from "@tauri-apps/api/core";
import { listen, type UnlistenFn } from "@tauri-apps/api/event";
import type { Method, Methods, Notification, ServeError } from "./protocol";

// Ids start from the clock so a webview reload never reuses an id that the
// previous page still has in flight.
let nextId = Date.now() * 1000;

export class ServeCallError extends Error {
  constructor(
    readonly method: Method,
    readonly error: ServeError,
  ) {
    super(error.message);
  }
}

export interface Started<M extends Method> {
  id: number;
  result: Promise<Methods[M]["result"]>;
}

/** Starts a call and returns its id at once; `result` settles later. */
export function start<M extends Method>(method: M, params: Methods[M]["params"]): Started<M> {
  const id = nextId++;
  const result = invoke<Methods[M]["result"]>("serve_call", { id, method, params }).catch((e: unknown) => {
    throw new ServeCallError(method, toServeError(e));
  });
  return { id, result };
}

/** Calls a method and waits for its result. */
export function call<M extends Method>(method: M, params: Methods[M]["params"]): Promise<Methods[M]["result"]> {
  return start(method, params).result;
}

/** Stops an in-flight call or subscription. Unknown ids are ignored. */
export async function cancel(id: number): Promise<void> {
  await call("cancel", { id }).catch(() => undefined);
}

export function onNotification(fn: (n: Notification) => void): Promise<UnlistenFn> {
  return listen<Notification>("serve_notification", (e) => fn(e.payload));
}

export type SidecarStatus =
  | { state: "starting" }
  | { state: "running"; generation: number }
  | { state: "down"; reason: string; stderr: string[] };

export function sidecarStatus(): Promise<SidecarStatus> {
  return invoke<SidecarStatus>("serve_status");
}

export function onSidecarStatus(fn: (s: SidecarStatus) => void): Promise<UnlistenFn> {
  return listen<SidecarStatus>("serve_status", (e) => fn(e.payload));
}

export function onSidecarRestarted(fn: () => void): Promise<UnlistenFn> {
  return listen("serve_restarted", () => fn());
}

export function retrySidecar(): Promise<void> {
  return invoke("serve_retry");
}

function toServeError(e: unknown): ServeError {
  if (e && typeof e === "object" && "code" in e && "message" in e) return e as ServeError;
  return { code: -32000, message: String(e) };
}
