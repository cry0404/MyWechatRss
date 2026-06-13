#!/usr/bin/env python3
import json
import select
import subprocess
import sys
import time


SERVER = "/Applications/Proxyman.app/Contents/MacOS/mcp-server"


def send(proc, obj):
    proc.stdin.write(json.dumps(obj, separators=(",", ":")) + "\n")
    proc.stdin.flush()


def read_until_id(proc, target_id, timeout=10):
    end = time.time() + timeout
    stderr = []
    while time.time() < end:
        ready, _, _ = select.select([proc.stdout, proc.stderr], [], [], 0.2)
        for stream in ready:
            line = stream.readline()
            if not line:
                continue
            if stream is proc.stderr:
                stderr.append(line.rstrip())
                continue
            try:
                msg = json.loads(line)
            except json.JSONDecodeError:
                continue
            if msg.get("id") == target_id:
                return msg, stderr
    raise TimeoutError(f"timed out waiting for MCP response id={target_id}; stderr={stderr}")


def main():
    if len(sys.argv) < 2:
        print("usage: proxyman_mcp_call.py <tool_name> [json_arguments]", file=sys.stderr)
        return 2
    tool = sys.argv[1]
    args = json.loads(sys.argv[2]) if len(sys.argv) > 2 else {}

    proc = subprocess.Popen(
        [SERVER],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        bufsize=1,
    )
    try:
        send(proc, {
            "jsonrpc": "2.0",
            "id": 1,
            "method": "initialize",
            "params": {
                "protocolVersion": "2024-11-05",
                "capabilities": {},
                "clientInfo": {"name": "codex-proxyman-debug", "version": "1.0"},
            },
        })
        read_until_id(proc, 1)
        send(proc, {"jsonrpc": "2.0", "method": "notifications/initialized", "params": {}})
        send(proc, {
            "jsonrpc": "2.0",
            "id": 2,
            "method": "tools/call",
            "params": {"name": tool, "arguments": args},
        })
        msg, stderr = read_until_id(proc, 2)
        if stderr:
            print(json.dumps({"stderr": stderr}, ensure_ascii=False), file=sys.stderr)
        print(json.dumps(msg, ensure_ascii=False, indent=2))
    finally:
        proc.terminate()


if __name__ == "__main__":
    raise SystemExit(main())
