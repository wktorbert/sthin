import { Level, type LogLine } from "@/serve/protocol";

/** The TUI keeps the same number of lines; both Surfaces behave alike. */
export const LOG_CAPACITY = 50_000;

export interface LogFilter {
  /** Case-insensitive substring of the message or the process. */
  text: string;
  minLevel: Level;
}

/** A bounded buffer of log lines: the newest `capacity` lines survive. */
export class LogBuffer {
  private buf: LogLine[] = [];
  /** How many lines fell off the front since the last clear. */
  dropped = 0;

  constructor(readonly capacity = LOG_CAPACITY) {}

  add(batch: LogLine[]): void {
    this.buf.push(...batch);
    const over = this.buf.length - this.capacity;
    if (over > 0) {
      this.buf.splice(0, over);
      this.dropped += over;
    }
  }

  lines(): readonly LogLine[] {
    return this.buf;
  }

  clear(): void {
    this.buf = [];
    this.dropped = 0;
  }
}

export function matches(l: LogLine, f: LogFilter): boolean {
  if (l.level < f.minLevel) return false;
  if (!f.text) return true;
  const t = f.text.toLowerCase();
  return l.message.toLowerCase().includes(t) || l.process.toLowerCase().includes(t);
}
