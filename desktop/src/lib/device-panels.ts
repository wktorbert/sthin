import type { Device, Platform, PlatformStatus } from "@/serve/protocol";

export type PanelKey = Platform | "physical";

export interface Panel {
  key: PanelKey;
  title: string;
  devices: Device[];
}

const titles: Record<PanelKey, string> = {
  ios: "iOS Simulators",
  android: "Android Emulators",
  physical: "Physical Devices",
};

const platformName: Record<Platform, string> = { ios: "iOS", android: "Android" };

/**
 * Splits a listing into the window's panels, as the TUI does: one per virtual
 * platform, then physical devices. An unavailable platform (iOS on Windows)
 * has no panel; its reason becomes a notice shown once under the header.
 */
export function groupPanels(platforms: PlatformStatus[], devices: Device[]): { panels: Panel[]; notices: string[] } {
  const panels: Panel[] = [];
  const notices: string[] = [];
  for (const p of platforms) {
    if (!p.available) {
      if (p.reason) notices.push(p.reason);
      continue;
    }
    if (p.error) notices.push(`${platformName[p.platform]}: ${p.error}`);
    panels.push({ key: p.platform, title: titles[p.platform], devices: devices.filter((d) => d.platform === p.platform && d.kind !== "physical") });
  }
  panels.push({ key: "physical", title: titles.physical, devices: devices.filter((d) => d.kind === "physical") });
  return { panels, notices };
}
