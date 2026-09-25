import type { Initialize } from "@/serve/protocol";

/** The Serve schema this Desktop was built against. */
export const SCHEMA_VERSION = 1;

export type Gate = { ok: true } | { ok: false; reason: string };

/**
 * The Desktop bundles its own `lean`, so a mismatch means a packaging bug.
 * Refusing loudly beats a silent protocol drift.
 */
export function checkSidecar(appVersion: string, init: Initialize): Gate {
  if (init.schema_version !== SCHEMA_VERSION) {
    return {
      ok: false,
      reason: `The bundled lean speaks schema ${init.schema_version}; this app speaks schema ${SCHEMA_VERSION}.`,
    };
  }
  if (init.lean_version !== appVersion) {
    return {
      ok: false,
      reason: `The bundled lean is ${init.lean_version} but this app is ${appVersion}. Reinstall Lean.`,
    };
  }
  return { ok: true };
}
