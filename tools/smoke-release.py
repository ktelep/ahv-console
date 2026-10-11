#!/usr/bin/env python3
"""Execute the exact packaged binary for the current native release target."""
import argparse
import importlib.util
import json
from pathlib import Path
import platform
import tempfile

TOOLS = Path(__file__).resolve().parent


def load(name, filename):
    spec = importlib.util.spec_from_file_location(name, TOOLS / filename)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    args = parser.parse_args()
    verifier = load("release_verifier", "verify-release.py")
    smoke = load("native_smoke", "native-smoke.py")
    release = verifier.resolve_release_directory(args.directory)
    manifest = verifier.verify(release)
    goos = {"Windows": "windows", "Darwin": "darwin", "Linux": "linux"}.get(platform.system())
    arch = {"x86_64": "amd64", "amd64": "amd64", "arm64": "arm64", "aarch64": "arm64"}.get(platform.machine().lower())
    targets = [x for x in manifest["supportedTargets"] if (x["os"], x["architecture"]) == (goos, arch)]
    if len(targets) != 1:
        raise SystemExit("Current OS/architecture is not a supported release target")
    target = targets[0]
    entries = verifier.unpack(release / target["archive"])
    executable = "ahv-console.exe" if goos == "windows" else "ahv-console"
    name = f"ahv-console_{manifest['version']}_{goos}_{arch}/" + executable
    with tempfile.TemporaryDirectory(prefix="ahv-console-release-smoke-") as directory:
        binary = Path(directory) / executable
        binary.write_bytes(entries[name])
        binary.chmod(0o755)
        result = smoke.inspect(binary, manifest["version"])
    if result["sha256"] != target["binarySha256"]:
        raise SystemExit("Native startup check did not execute the manifest's binary")
    result["archive"] = target["archive"]
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()
