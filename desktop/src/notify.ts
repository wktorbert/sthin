// OS notifications for operations started from the Tray: once its menu closes
// there is no other feedback. Actions started in the window show progress
// inline and never notify.

import { isPermissionGranted, requestPermission, sendNotification } from "@tauri-apps/plugin-notification";

let allowed: boolean | undefined;

async function permitted(): Promise<boolean> {
  if (allowed === undefined) {
    allowed = (await isPermissionGranted()) || (await requestPermission()) === "granted";
  }
  return allowed;
}

export async function notifyDone(kind: "boot" | "restore", name: string, error?: string): Promise<void> {
  if (!(await permitted())) return;
  const verb = kind === "boot" ? "Boot" : "Restore";
  sendNotification(
    error
      ? { title: `${verb} failed: ${name}`, body: error }
      : { title: kind === "boot" ? `${name} is ready` : `${name} restored to stock`, body: kind === "boot" ? "Slim boot finished." : "Everything Sthin changed was undone." },
  );
}
