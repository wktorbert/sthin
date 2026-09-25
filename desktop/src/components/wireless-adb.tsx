// Wireless ADB: pair once with the phone's code, then connect to its main
// address. Same flow as `lean adb` and the TUI's `w`.

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { call } from "@/serve/client";
import { message } from "@/store";

export function WirelessADB({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const [pairAddr, setPairAddr] = useState("");
  const [code, setCode] = useState("");
  const [addr, setAddr] = useState("");
  const [result, setResult] = useState<{ ok: boolean; text: string }>();
  const [busy, setBusy] = useState(false);

  const run = async (p: Promise<{ message: string }>) => {
    setBusy(true);
    try {
      setResult({ ok: true, text: (await p).message });
    } catch (e) {
      setResult({ ok: false, text: message(e) });
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Wireless ADB</DialogTitle>
          <DialogDescription>On the phone: Developer options › Wireless debugging. Pair once, then connect.</DialogDescription>
        </DialogHeader>
        <form className="grid gap-2" onSubmit={(e) => (e.preventDefault(), run(call("adb_pair", { addr: pairAddr, code })))}>
          <Label htmlFor="pair-addr">Pairing address and code</Label>
          <div className="flex gap-2">
            <Input id="pair-addr" placeholder="192.168.1.20:37099" value={pairAddr} onChange={(e) => setPairAddr(e.target.value)} />
            <Input aria-label="Pairing code" className="w-28" placeholder="123456" inputMode="numeric" value={code} onChange={(e) => setCode(e.target.value)} />
            <Button type="submit" disabled={busy || !pairAddr || !code}>
              Pair
            </Button>
          </div>
        </form>
        <form className="grid gap-2" onSubmit={(e) => (e.preventDefault(), run(call("adb_connect", { addr })))}>
          <Label htmlFor="addr">Device address</Label>
          <div className="flex gap-2">
            <Input id="addr" placeholder="192.168.1.20:5555" value={addr} onChange={(e) => setAddr(e.target.value)} />
            <Button type="submit" disabled={busy || !addr}>
              Connect
            </Button>
            <Button type="button" variant="outline" disabled={busy || !addr} onClick={() => run(call("adb_disconnect", { addr }))}>
              Disconnect
            </Button>
          </div>
        </form>
        {result && <p className={result.ok ? "text-sm text-success select-text" : "text-sm text-destructive select-text"}>{result.text}</p>}
      </DialogContent>
    </Dialog>
  );
}
