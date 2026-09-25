#!/usr/bin/env python3
"""Minimal `lean serve` stdio client for smoke tests: speaks the same
newline-delimited JSON-RPC 2.0 the desktop app does.

Usage: serve-client.py <lean-binary> <method> [json-params] [<method> [json-params] ...]
Always calls `initialize` first. Prints one JSON object per call:
{"method": ..., "result": ...} or {"method": ..., "error": ...}.
Notifications that arrive meanwhile are printed as {"notification": ..., "params": ...}.
Exit 1 if any call returns an error or the session closes early."""
import json
import subprocess
import sys


def main():
    if len(sys.argv) < 2:
        print(__doc__)
        sys.exit(2)
    binary, rest = sys.argv[1], sys.argv[2:]
    proc = subprocess.Popen([binary, "serve"], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE, text=True)
    next_id = [0]

    def call(method, params=None):
        next_id[0] += 1
        msg = {"jsonrpc": "2.0", "id": next_id[0], "method": method}
        if params is not None:
            msg["params"] = params
        proc.stdin.write(json.dumps(msg) + "\n")
        proc.stdin.flush()
        while True:
            line = proc.stdout.readline()
            if not line:
                raise SystemExit("serve closed: " + proc.stderr.read())
            m = json.loads(line)
            if "method" in m:
                print(json.dumps({"notification": m["method"], "params": m.get("params")}), flush=True)
                continue
            if m.get("id") == next_id[0]:
                return m

    calls = [("initialize", {"client_name": "serve-client.py", "client_version": "0"})]
    i = 0
    while i < len(rest):
        method, params = rest[i], None
        if i + 1 < len(rest) and rest[i + 1].startswith("{"):
            params = json.loads(rest[i + 1])
            i += 1
        i += 1
        calls.append((method, params))

    failed = False
    for method, params in calls:
        resp = call(method, params)
        if "error" in resp:
            failed = True
            print(json.dumps({"method": method, "error": resp["error"]}), flush=True)
        else:
            print(json.dumps({"method": method, "result": resp["result"]}), flush=True)
    proc.stdin.close()
    proc.wait(timeout=10)
    sys.exit(1 if failed else 0)


if __name__ == "__main__":
    main()
