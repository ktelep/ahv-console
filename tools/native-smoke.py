#!/usr/bin/env python3
"""Build or inspect the current native target and run offline release smoke checks."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]


def inspect(binary, version):
    env = {key: value for key, value in os.environ.items() if not key.startswith("AHV_CONSOLE_")}
    output = {}
    for flag in ("--version", "--help", "--licenses"):
        result = subprocess.run([str(binary), flag], capture_output=True, text=True, encoding="utf-8", env=env, timeout=20, check=True)
        if not result.stdout or result.stderr:
            raise ValueError(f"Unexpected offline output for {flag}")
        output[flag] = result.stdout
    if output["--version"].strip() != "ahv-console " + version:
        raise ValueError("Release binary version mismatch")
    for notice in ("Copyright (c) 2026 Kurt Telep", "Mozilla Public License", "Vitaly Puzrin", "Jef Poskanzer", "Widget Workshop", "2025 Coder"):
        if notice not in output["--licenses"]:
            raise ValueError(f"Embedded notice missing: {notice}")
    return {"version": version, "os": platform.system(), "architecture": platform.machine(), "sha256": hashlib.sha256(binary.read_bytes()).hexdigest(), "offlineCommands": "passed", "prismContactRequired": False}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path)
    parser.add_argument("--version", default=(ROOT / "VERSION").read_text().strip())
    args = parser.parse_args()
    if args.binary:
        result = inspect(args.binary.resolve(), args.version)
    else:
        with tempfile.TemporaryDirectory(prefix="ahv-console-smoke-") as directory:
            binary = Path(directory) / ("ahv-console.exe" if os.name == "nt" else "ahv-console")
            env = dict(os.environ, CGO_ENABLED="0", GOTOOLCHAIN="local")
            subprocess.run(["go", "build", "-trimpath", "-buildvcs=false", "-ldflags", "-X ahv-console/internal/cli.Version=" + args.version, "-o", str(binary), "./cmd/ahv-console"], cwd=ROOT, env=env, check=True)
            result = inspect(binary, args.version)
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()
