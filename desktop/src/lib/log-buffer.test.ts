import { describe, expect, it } from "vitest";
import { Level, type LogLine } from "@/serve/protocol";
import { LogBuffer, matches } from "./log-buffer";

const line = (message: string, level = Level.Info, process = "app"): LogLine => ({
  time: "01:00:00",
  level,
  process,
  message,
  raw: message,
});

describe("LogBuffer", () => {
  it("keeps the newest lines up to its capacity and counts what it dropped", () => {
    const b = new LogBuffer(3);
    b.add([line("a"), line("b")]);
    b.add([line("c"), line("d"), line("e")]);
    expect(b.lines().map((l) => l.message)).toEqual(["c", "d", "e"]);
    expect(b.dropped).toBe(2);
  });

  it("takes a batch larger than its capacity", () => {
    const b = new LogBuffer(2);
    b.add([line("a"), line("b"), line("c")]);
    expect(b.lines().map((l) => l.message)).toEqual(["b", "c"]);
  });

  it("clears", () => {
    const b = new LogBuffer(2);
    b.add([line("a")]);
    b.clear();
    expect(b.lines()).toEqual([]);
    expect(b.dropped).toBe(0);
  });
});

describe("matches", () => {
  it("filters by minimum level", () => {
    expect(matches(line("x", Level.Warn), { text: "", minLevel: Level.Error })).toBe(false);
    expect(matches(line("x", Level.Fatal), { text: "", minLevel: Level.Error })).toBe(true);
  });

  it("filters by text in the message or the process, ignoring case", () => {
    const f = { text: "BOOM", minLevel: Level.Verbose };
    expect(matches(line("big boom here"), f)).toBe(true);
    expect(matches(line("quiet", Level.Info, "boomd"), f)).toBe(true);
    expect(matches(line("quiet"), f)).toBe(false);
  });
});
