import type { Stage } from "@/serve/protocol";

/**
 * Folds one progress notification into the stage list: an outcome replaces the
 * same stage's `running` entry when that entry is the last one, otherwise the
 * stage is appended. Returns a new array.
 */
export function applyStage(stages: readonly Stage[], s: Stage): Stage[] {
  const last = stages[stages.length - 1];
  if (last && last.name === s.name && last.status === "running") {
    return [...stages.slice(0, -1), s];
  }
  return [...stages, s];
}

export function failedStage(stages: readonly Stage[]): Stage | undefined {
  return stages.find((s) => s.status === "fail");
}
