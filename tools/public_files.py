"""Shared allowlist, notice integrity, and public-distribution checks."""
import hashlib
import io
from pathlib import Path
import re
import tarfile

ROOT = Path(__file__).resolve().parents[1]
NOVNC_SHA256 = "ebb8eabcd3670ce1a02e4025f03976826c289121cf631183647bd8325b003403"
NOVNC_SOURCE = "internal/viewer/assets/novnc/SOURCE.tar.gz"
ROOT_FILES = (
    ".gitignore", ".gitattributes", "LICENSE", "VERSION", "README.md", "CHANGELOG.md",
    "THIRD_PARTY_NOTICES.md", "CONTRIBUTING.md", "SECURITY.md", "go.mod", "go.sum",
)
PUBLIC_DOCS = (
    "docs/asset-provenance.md", "docs/pc-7.6-compatibility.md",
    "docs/publishing.md", "docs/verification/release-0.1.0.md",
    "docs/verification/linux-arm64-test.md", "docs/verification/pc-7.6-handshake.json",
)
PUBLIC_DIRS = (".github", "cmd", "internal", "testdata", "tools")
REQUIRED_NOTICES = (
    "AHV-Console-LICENSE.txt", "Go-LICENSE.txt",
    "github.com-coder-websocket-v1.8.15.txt", "golang.org-x-term-v0.47.0.txt",
    "golang.org-x-sys-v0.49.0.txt", "noVNC-LICENSE.txt", "noVNC-AUTHORS",
    "noVNC-LICENSE.MPL-2.0", "noVNC-LICENSE.BSD-2-Clause",
    "noVNC-LICENSE.BSD-3-Clause", "noVNC-LICENSE.OFL-1.1",
    "pako-LICENSE-MIT.txt", "noVNC-DES-attribution.txt",
)
SECRET_PATTERNS = (
    ("private key", re.compile(rb"-----BEGIN (?:[A-Z0-9 ]+ )?PRIVATE KEY-----")),
    ("JWT", re.compile(rb"\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b")),
    ("GitHub token", re.compile(rb"\b(?:gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{30,})\b")),
)
PRIVATE_DATA_PATTERNS = (
    ("private lab IPv4", re.compile(rb"\b(?:192\.168\.\d{1,3}\.\d{1,3}|10\.\d{1,3}\.\d{1,3}\.\d{1,3}|172\.(?:1[6-9]|2\d|3[01])\.\d{1,3}\.\d{1,3})\b")),
    ("local workspace path", re.compile(rb"/home/[^/\s]+/Workspace/")),
)


def sha256(data):
    return hashlib.sha256(data).hexdigest()


def source_files(root=ROOT):
    root = Path(root)
    paths = {root / path for path in ROOT_FILES + PUBLIC_DOCS}
    for directory in PUBLIC_DIRS:
        parent = root / directory
        if not parent.is_dir() or parent.is_symlink():
            raise ValueError(f"Missing or unsafe public directory: {directory}")
        for path in parent.rglob("*"):
            if path.is_symlink():
                raise ValueError(f"Symlink in public tree: {path.relative_to(root)}")
            if path.is_file() and "__pycache__" not in path.parts and path.suffix != ".pyc":
                paths.add(path)
    for path in paths:
        if path.is_symlink() or not path.is_file():
            raise ValueError(f"Missing or unsafe public file: {path.relative_to(root)}")
    return sorted(paths, key=lambda path: path.relative_to(root).as_posix())


def check_content(name, data, vendored=False):
    for label, pattern in SECRET_PATTERNS + (() if vendored else PRIVATE_DATA_PATTERNS):
        if pattern.search(data):
            # Never reproduce the matching secret in diagnostics.
            raise ValueError(f"Public file contains {label}: {name}")


def verify_notices(root=ROOT):
    root = Path(root)
    notices = root / "internal/viewer/notices"
    for name in REQUIRED_NOTICES:
        path = notices / name
        if not path.is_file() or not path.read_bytes().strip():
            raise ValueError(f"Missing required notice: {name}")
    if (notices / "AHV-Console-LICENSE.txt").read_bytes() != (root / "LICENSE").read_bytes():
        raise ValueError("Embedded original-code license differs from LICENSE")
    assets = root / "internal/viewer/assets/novnc"
    if (notices / "pako-LICENSE-MIT.txt").read_bytes() != (assets / "vendor/pako/LICENSE").read_bytes():
        raise ValueError("pako notice differs from the upstream license")
    des = (assets / "core/crypto/des.js").read_text()
    expected = des.split("*/", 1)[0] + "*/\n"
    if (notices / "noVNC-DES-attribution.txt").read_text() != expected:
        raise ValueError("DES attribution differs from the upstream header")


def verify_vendored_source(root=ROOT):
    root = Path(root)
    data = (root / NOVNC_SOURCE).read_bytes()
    if sha256(data) != NOVNC_SHA256:
        raise ValueError("Embedded noVNC source archive checksum mismatch")
    expected = {}
    with tarfile.open(fileobj=io.BytesIO(data), mode="r:gz") as archive:
        for member in archive:
            parts = Path(member.name).parts[1:]
            if member.isfile() and parts and parts[0] in ("core", "vendor"):
                expected["/".join(parts)] = archive.extractfile(member).read()
    assets = root / "internal/viewer/assets/novnc"
    actual = {path.relative_to(assets).as_posix(): path.read_bytes() for directory in ("core", "vendor") for path in (assets / directory).rglob("*") if path.is_file()}
    if actual != expected:
        raise ValueError("Bundled noVNC files differ from the matching Source Code Form")


def verify_source(root=ROOT):
    root = Path(root)
    verify_notices(root)
    verify_vendored_source(root)
    files = source_files(root)
    for path in files:
        name = path.relative_to(root).as_posix()
        if path.suffix.lower() in (".pem", ".key", ".p12", ".pfx", ".log") or path.name.startswith(".env"):
            raise ValueError(f"Private artifact type in public export: {name}")
        if name != NOVNC_SOURCE:
            check_content(name, path.read_bytes(), vendored=name.startswith("internal/viewer/assets/novnc/"))
    return files
