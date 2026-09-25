import { describe, expect, it } from "vitest";
import type { Stage } from "@/serve/protocol";
import { applyStage, failedStage } from "./stage-progress";

const st = (name: string, status: Stage["status"], detail?: string): Stage => ({ name, status, detail });

describe("applyStage", () => {
  it("replaces a running stage with its outcome and keeps order", () => {
    let s: Stage[] = [];
    s = applyStage(s, st("tune", "running"));
    s = applyStage(s, st("tune", "ok", "3 labels"));
    s = applyStage(s, st("boot", "running"));
    expect(s).toEqual([st("tune", "ok", "3 labels"), st("boot", "running")]);
  });

  it("appends a stage that was never reported running", () => {
    const s = applyStage([st("tune", "ok")], st("open-simulator", "warn", "no window"));
    expect(s.map((x) => x.name)).toEqual(["tune", "open-simulator"]);
  });

  it("does not replace a finished stage of the same name", () => {
    const s = applyStage([st("measure", "ok")], st("measure", "running"));
    expect(s).toHaveLength(2);
  });
});

describe("failedStage", () => {
  it("returns the failed stage, if any", () => {
    expect(failedStage([st("tune", "ok"), st("boot", "fail", "timed out")])?.name).toBe("boot");
    expect(failedStage([st("tune", "ok")])).toBeUndefined();
  });
});
