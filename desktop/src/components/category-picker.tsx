// Choose which Categories stay enabled on this device, then slim boot. Opens
// pre-ticked with the device's saved choice; saving it is on by default, as in
// the TUI's `c`.

import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { call } from "@/serve/client";
import type { Category, Device } from "@/serve/protocol";
import { bootDevice, message } from "@/store";

interface Props {
  device: Device;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function CategoryPicker({ device, open, onOpenChange }: Props) {
  const [categories, setCategories] = useState<Category[]>([]);
  const [keep, setKeep] = useState<Set<string>>(new Set());
  const [remember, setRemember] = useState(true);
  const [error, setError] = useState<string>();

  useEffect(() => {
    if (!open) return;
    setError(undefined);
    Promise.all([call("profile", { platform: device.platform }), call("prefs_get", { id: device.id })])
      .then(([profile, prefs]) => {
        // Aggressive categories are off by default and hidden, as in the TUI.
        setCategories(profile.categories.filter((c) => c.default));
        setKeep(new Set(prefs.except));
      })
      .catch((e) => setError(message(e)));
  }, [open, device.id, device.platform]);

  const toggle = (id: string, on: boolean) => {
    const next = new Set(keep);
    if (on) next.add(id);
    else next.delete(id);
    setKeep(next);
  };

  const boot = () => {
    onOpenChange(false);
    void bootDevice(device.id, { except: [...keep], remember });
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Keep enabled on {device.name}</DialogTitle>
          <DialogDescription>Ticked categories stay on. Everything else in the Profile is disabled by the slim boot.</DialogDescription>
        </DialogHeader>
        {error ? (
          <p className="text-sm text-destructive">{error}</p>
        ) : (
          <ul className="max-h-80 space-y-2 overflow-auto pr-2">
            {categories.map((c) => (
              <li key={c.id} className="flex items-center gap-3">
                <Checkbox id={`keep-${c.id}`} checked={keep.has(c.id)} onCheckedChange={(v) => toggle(c.id, v === true)} />
                <Label htmlFor={`keep-${c.id}`} className="flex flex-1 justify-between font-normal">
                  <span>
                    {c.name} <span className="font-mono text-xs text-muted-foreground">{c.id}</span>
                  </span>
                  <span className="text-xs text-muted-foreground tabular-nums">
                    {c.items.length} items{c.approx_mb ? ` · ~${c.approx_mb} MB` : ""}
                  </span>
                </Label>
              </li>
            ))}
          </ul>
        )}
        <DialogFooter className="items-center gap-3 sm:justify-between">
          <div className="flex items-center gap-2">
            <Checkbox id="remember" checked={remember} onCheckedChange={(v) => setRemember(v === true)} />
            <Label htmlFor="remember" className="font-normal">
              Save as this device's default
            </Label>
          </div>
          <Button onClick={boot} disabled={!!error}>
            Slim boot
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
