#!/usr/bin/env python3
"""Minimal MCP stdio client for smoke tests and scripts: speaks JSON-RPC to
`lean mcp` the same way Claude Code does.

Usage: mcp-client.py <lean-binary> <tool> [json-args] [<tool> [json-args] ...]
Prints one JSON object per call: {"tool": ..., "isError": ..., "text": ...}.
Exit 1 if any call is a tool error or the handshake fails."""
import json
import subprocess
import sys


def main():
    if len(sys.argv) < 3:
        print(__doc__)
        sys.exit(2)
    binary, rest = sys.argv[1], sys.argv[2:]
    proc = subprocess.Popen([binary, "mcp"], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE, text=True)
    ids = iter(range(1, 10_000))

    def send(method, params=None, notify=False):
        msg = {"jsonrpc": "2.0", "method": method}
        if params is not None:
            msg["params"] = params
        if not notify:
            msg["id"] = next(ids)
        proc.stdin.write(json.dumps(msg) + "\n")
        proc.stdin.flush()
        if notify:
            return None
        while True:
            line = proc.stdout.readline()
            if not line:
                raise SystemExit("server closed: " + proc.stderr.read())
            resp = json.loads(line)
            if resp.get("id") == msg["id"]:
                if "error" in resp:
                    raise SystemExit("rpc error: " + json.dumps(resp["error"]))
                return resp["result"]

    init = send("initialize", {"protocolVersion": "2025-06-18", "capabilities": {},
                               "clientInfo": {"name": "mcp-client.py", "version": "0"}})
    send("notifications/initialized", notify=True)
    tools = [t["name"] for t in send("tools/list")["tools"]]
    print(json.dumps({"server": init["serverInfo"], "tools": tools}), flush=True)
    failed = False
    i = 0
    while i < len(rest):
        tool = rest[i]
        args = {}
        if i + 1 < len(rest) and rest[i + 1].startswith("{"):
            args = json.loads(rest[i + 1])
            i += 1
        i += 1
        res = send("tools/call", {"name": tool, "arguments": args})
        text = "".join(c.get("text", "") for c in res.get("content", []) if c.get("type") == "text")
        is_err = bool(res.get("isError"))
        failed = failed or is_err
        print(json.dumps({"tool": tool, "isError": is_err, "text": text}), flush=True)
    proc.stdin.close()
    proc.wait(timeout=10)
    sys.exit(1 if failed else 0)


if __name__ == "__main__":
    main()
