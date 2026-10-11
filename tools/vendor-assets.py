#!/usr/bin/env python3
"""Reproduce embedded upstream assets and notices (build time only)."""
import hashlib
import io
import json
from pathlib import Path, PurePosixPath
import subprocess
import tarfile
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
COMMIT = "a8dfd6a3ea3c74244f5ebdaa5a7f1023007a7820"
SHA256 = "ebb8eabcd3670ce1a02e4025f03976826c289121cf631183647bd8325b003403"
URL = f"https://codeload.github.com/novnc/noVNC/tar.gz/{COMMIT}"
ASSETS = ROOT / "internal/viewer/assets/novnc"
NOTICES = ROOT / "internal/viewer/notices"


def main():
    archive = urllib.request.urlopen(URL, timeout=60).read()
    if hashlib.sha256(archive).hexdigest() != SHA256:
        raise SystemExit("noVNC source checksum mismatch")
    ASSETS.mkdir(parents=True, exist_ok=True)
    NOTICES.mkdir(parents=True, exist_ok=True)
    (ASSETS / "SOURCE.tar.gz").write_bytes(archive)
    with tarfile.open(fileobj=io.BytesIO(archive), mode="r:gz") as source:
        for member in source:
            path = PurePosixPath(member.name)
            parts = path.parts[1:]
            if not member.isfile() or not parts or ".." in parts:
                continue
            relative = PurePosixPath(*parts)
            if parts[0] in ("core", "vendor"):
                dest = ASSETS.joinpath(*parts)
            elif str(relative) in ("LICENSE.txt", "AUTHORS") or str(relative).startswith("docs/LICENSE."):
                dest = NOTICES / ("noVNC-" + relative.name)
            else:
                continue
            dest.parent.mkdir(parents=True, exist_ok=True)
            dest.write_bytes(source.extractfile(member).read())
    (NOTICES / "pako-LICENSE-MIT.txt").write_bytes((ASSETS / "vendor/pako/LICENSE").read_bytes())
    des = (ASSETS / "core/crypto/des.js").read_text()
    if not des.startswith("/*") or "*/" not in des:
        raise SystemExit("Expected DES attribution header not found")
    (NOTICES / "noVNC-DES-attribution.txt").write_text(des.split("*/", 1)[0] + "*/\n")
    (NOTICES / "AHV-Console-LICENSE.txt").write_bytes((ROOT / "LICENSE").read_bytes())
    for module in ("github.com/coder/websocket", "golang.org/x/term", "golang.org/x/sys"):
        info = json.loads(subprocess.check_output(["go", "mod", "download", "-json", module], cwd=ROOT))
        module_root = Path(info["Dir"])
        name = module.replace("/", "-") + "-" + info["Version"]
        license_file = module_root / "LICENSE"
        if not license_file.exists():
            license_file = module_root / "LICENSE.txt"
        (NOTICES / (name + ".txt")).write_bytes(license_file.read_bytes())
    goroot = Path(subprocess.check_output(["go", "env", "GOROOT"], cwd=ROOT, text=True).strip())
    go_license = goroot / "LICENSE"
    if not go_license.exists():
        go_license = Path("/usr/share/licenses/golang/LICENSE")
    (NOTICES / "Go-LICENSE.txt").write_bytes(go_license.read_bytes())
    print(f"Verified and embedded noVNC 1.6.0 ({COMMIT}) and Go dependency notices")


if __name__ == "__main__":
    main()
