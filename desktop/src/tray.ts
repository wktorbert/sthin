// Binds the Tray (created in Rust with id "main") to the store: the menu is
// rebuilt from lib/tray-model whenever devices or in-flight operations change.

import { invoke } from "@tauri-apps/api/core";
import { Menu, MenuItem, PredefinedMenuItem, Submenu } from "@tauri-apps/api/menu";
import { TrayIcon } from "@tauri-apps/api/tray";
import { parseTrayAction, trayModel, type TrayEntry } from "@/lib/tray-model";
import { bootDevice, restoreDevice, select, setView, shutdownDevice, useLean } from "@/store";

export async function bindTray(): Promise<void> {
  const tray = await TrayIcon.getById("main");
  if (!tray) return;
  let last = "";
  // Renders run one at a time, each reading the latest state, so an older
  // menu can never be applied after a newer one.
  let queue = Promise.resolve();
  const schedule = () => {
    queue = queue.then(render).catch(() => undefined);
  };
  const render = async () => {
    const s = useLean.getState();
    const busy = new Set(Object.entries(s.ops).filter(([, op]) => op.running).map(([id]) => id));
    const model = s.phase === "ready" ? trayModel(s.platforms, s.devices, busy) : trayModel([], [], busy);
    const key = JSON.stringify(model);
    if (key === last) return;
    await tray.setMenu(await build(model));
    last = key;
  };
  useLean.subscribe(schedule);
  schedule();
  await queue;
}

async function build(model: TrayEntry[]): Promise<Menu> {
  const items = [];
  for (const e of model) {
    switch (e.type) {
      case "header":
      case "empty":
        items.push(await MenuItem.new({ text: e.text, enabled: false }));
        break;
      case "separator":
        items.push(await PredefinedMenuItem.new({ item: "Separator" }));
        break;
      case "action":
        items.push(await MenuItem.new({ id: e.id, text: e.text, action: act }));
        break;
      case "device":
        items.push(
          await Submenu.new({
            text: e.text,
            items: await Promise.all(e.items.map((a) => MenuItem.new({ id: a.id, text: a.text, enabled: a.enabled, action: act }))),
          }),
        );
    }
  }
  return Menu.new({ items });
}

function act(itemId: string): void {
  const { verb, id } = parseTrayAction(itemId);
  switch (verb) {
    case "boot":
      void bootDevice(id, {}, true);
      return;
    case "restore":
      void restoreDevice(id, true);
      return;
    case "shutdown":
      void shutdownDevice(id);
      return;
    case "progress":
      select(id);
      void invoke("show_window");
      return;
    case "show":
      void invoke("show_window");
      return;
    case "doctor":
      setView("doctor");
      void invoke("show_window");
      return;
    case "quit":
      void invoke("quit_app");
  }
}
