#!/usr/bin/env python3
"""Build pinned single-file targets and checksums without CGO."""
import hashlib
import argparse
import os
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[1]
TARGETS = (("windows", "amd64"), ("darwin", "arm64"), ("linux", "amd64"), ("linux", "arm64"))


def validate_version(version):
    if not version or any(c not in "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._-" for c in version):
        raise SystemExit("Version must contain only letters, digits, dot, underscore, or hyphen")
    if version in (".", ".."):
        raise SystemExit("Invalid version directory")
    return version


def build(version, output=None, experimental=False):
    version = validate_version(version)
    output = Path(output) if output else ROOT / "dist" / version / "bin"
    output.mkdir(parents=True, exist_ok=True)
    sums = []
    for goos, arch in TARGETS:
        name = f"ahv-console-{goos}-{arch}" + (".exe" if goos == "windows" else "")
        binary = output / name
        env = dict(os.environ, GOOS=goos, GOARCH=arch, CGO_ENABLED="0", GOTOOLCHAIN="local")
        subprocess.run(["go", "build", "-trimpath", "-buildvcs=false", "-ldflags", f"-s -w -buildid= -X ahv-console/internal/cli.Version={version}", "-o", str(binary), "./cmd/ahv-console"], cwd=ROOT, env=env, check=True)
        sums.append(f"{hashlib.sha256(binary.read_bytes()).hexdigest()}  {name}")
        print(f"Built supported target {name}")
    (output / "SHA256SUMS").write_text("\n".join(sums) + "\n")
    if experimental:
        experimental_dir = output.parent / "experimental"
        experimental_dir.mkdir(exist_ok=True)
        subprocess.run(["go", "build", "-trimpath", "-buildvcs=false", "-ldflags", f"-s -w -buildid= -X ahv-console/internal/cli.Version={version}-experimental", "-o", str(experimental_dir / "ahv-console-darwin-amd64"), "./cmd/ahv-console"], cwd=ROOT, env=dict(os.environ, GOOS="darwin", GOARCH="amd64", CGO_ENABLED="0", GOTOOLCHAIN="local"), check=True)
        print("Built unqualified macOS amd64 separately; not included in supported checksums or release bundles")
    return output


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("version", nargs="?", default=(ROOT / "VERSION").read_text().strip())
    parser.add_argument("--experimental-macos-amd64", action="store_true")
    args = parser.parse_args()
    build(args.version, experimental=args.experimental_macos_amd64)


if __name__ == "__main__":
    main()
