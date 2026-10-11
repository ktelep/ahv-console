#!/usr/bin/env python3
"""Create a clean GitHub source export and compliant versioned release bundles."""
import argparse
import gzip
import io
import json
from pathlib import Path
import shutil
import subprocess
import tarfile
import zipfile

from build import ROOT, TARGETS, build, validate_version
from public_files import NOVNC_SOURCE, NOVNC_SHA256, sha256, verify_source


def tar_bytes(entries):
    stream = io.BytesIO()
    with gzip.GzipFile(filename="", mode="wb", fileobj=stream, mtime=0) as compressed:
        with tarfile.open(fileobj=compressed, mode="w", format=tarfile.PAX_FORMAT) as archive:
            for name, data, executable in sorted(entries):
                member = tarfile.TarInfo(name)
                member.size = len(data)
                member.mode = 0o755 if executable else 0o644
                member.mtime = 0
                archive.addfile(member, io.BytesIO(data))
    return stream.getvalue()


def zip_bytes(entries):
    stream = io.BytesIO()
    with zipfile.ZipFile(stream, "w", compression=zipfile.ZIP_DEFLATED) as archive:
        for name, data, executable in sorted(entries):
            member = zipfile.ZipInfo(name, date_time=(1980, 1, 1, 0, 0, 0))
            member.create_system = 3
            member.external_attr = ((0o100755 if executable else 0o100644) << 16)
            member.compress_type = zipfile.ZIP_DEFLATED
            archive.writestr(member, data)
    return stream.getvalue()


def export_source(root, output, files):
    source = output / "github"
    marker = output / "export-manifest.json"
    if source.exists():
        if source.is_symlink() or (source / ".git").exists() or not marker.is_file():
            raise ValueError("Refusing to overwrite an existing non-generated/Git source directory")
        previous = json.loads(marker.read_text())["files"]
        current = {path.relative_to(source).as_posix(): sha256(path.read_bytes()) for path in source.rglob("*") if path.is_file() and "__pycache__" not in path.parts and path.suffix != ".pyc"}
        if any(path.is_symlink() for path in source.rglob("*")) or current != previous:
            raise ValueError("Generated source export has local edits; use a new --output directory to preserve them")
        shutil.rmtree(source)
    source.mkdir(parents=True)
    hashes = {}
    for path in files:
        name = path.relative_to(root).as_posix()
        target = source / name
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(path, target)
        hashes[name] = sha256(target.read_bytes())
    marker.write_text(json.dumps({"schemaVersion": 1, "files": hashes}, indent=2, sort_keys=True) + "\n")
    verify_source(source)
    return source, hashes


def prepare(version, output, run_build=True):
    root = ROOT
    version = validate_version(version.removeprefix("v"))
    if version != (root / "VERSION").read_text().strip():
        raise ValueError("Release version must match VERSION; update VERSION and release documentation together")
    files = verify_source(root)
    candidate = Path(output)
    if candidate.is_symlink():
        raise ValueError("Unsafe publication output")
    output = candidate.resolve()
    if output == root or root.is_relative_to(output):
        raise ValueError("Publication output must not be the source root or its parent")
    if output.is_relative_to(root) and output.relative_to(root).parts[0] in (".github", "cmd", "internal", "testdata", "tools", "docs"):
        raise ValueError("Publication output must be outside allowlisted source directories")
    if output.is_symlink():
        raise ValueError("Unsafe publication output")
    output.mkdir(parents=True, exist_ok=True)
    source, source_hashes = export_source(root, output, files)
    binaries = build(version) if run_build else root / "dist" / version / "bin"
    release = output / "releases" / ("v" + version)
    release.mkdir(parents=True, exist_ok=True)
    existing = list(release.iterdir())
    if existing:
        checksum_file = release / "SHA256SUMS"
        if not checksum_file.is_file():
            raise ValueError("Refusing to overwrite an unrecognized release directory")
        previous = {name: digest for digest, name in (line.split() for line in checksum_file.read_text().splitlines())}
        actual = {path.name: sha256(path.read_bytes()) for path in existing if path.name != "SHA256SUMS" and path.is_file() and not path.is_symlink()}
        if actual != previous or len(existing) != len(previous) + 1:
            raise ValueError("Existing release files have edits or unexpected contents; use a fresh --output directory")
    common = [("LICENSE", (root / "LICENSE").read_bytes(), False), ("README.md", (root / "README.md").read_bytes(), False), ("THIRD_PARTY_NOTICES.md", (root / "THIRD_PARTY_NOTICES.md").read_bytes(), False)]
    common += [("licenses/" + path.name, path.read_bytes(), False) for path in sorted((root / "internal/viewer/notices").iterdir()) if path.is_file()]
    novnc = (root / NOVNC_SOURCE).read_bytes()
    common.append(("source/novnc-1.6.0-source.tar.gz", novnc, False))
    targets = []
    generated = {}
    for goos, arch in TARGETS:
        binary_name = f"ahv-console-{goos}-{arch}" + (".exe" if goos == "windows" else "")
        binary = (binaries / binary_name).read_bytes()
        folder = f"ahv-console_{version}_{goos}_{arch}"
        executable = "ahv-console.exe" if goos == "windows" else "ahv-console"
        entries = [(folder + "/" + name, data, mode) for name, data, mode in common + [(executable, binary, True)]]
        archive_name = folder + (".zip" if goos == "windows" else ".tar.gz")
        generated[archive_name] = zip_bytes(entries) if goos == "windows" else tar_bytes(entries)
        targets.append({"os": goos, "architecture": arch, "archive": archive_name, "binarySha256": sha256(binary), "archiveSha256": sha256(generated[archive_name])})
    source_name = f"ahv-console_{version}_source.tar.gz"
    generated[source_name] = tar_bytes([(f"ahv-console-{version}/" + path.relative_to(root).as_posix(), path.read_bytes(), False) for path in files])
    generated["novnc-1.6.0-source.tar.gz"] = novnc
    generated["RELEASE_NOTES.md"] = (root / "CHANGELOG.md").read_bytes()
    manifest = {"schemaVersion": 1, "version": version, "goToolchain": subprocess.check_output(["go", "version"], text=True).strip(), "supportedTargets": targets, "excludedTargets": [{"os": "darwin", "architecture": "amd64", "reason": "Unqualified; excluded by user-approved initial release scope"}], "originalCodeLicense": "MIT", "copyright": "Copyright (c) 2026 Kurt Telep", "sourceArchive": source_name, "sourceArchiveSha256": sha256(generated[source_name]), "publicSourceFiles": source_hashes, "noVNC": {"version": "1.6.0", "commit": "a8dfd6a3ea3c74244f5ebdaa5a7f1023007a7820", "sourceSha256": NOVNC_SHA256}, "qualificationRecord": f"docs/verification/release-{version}.md"}
    generated["release-manifest.json"] = (json.dumps(manifest, indent=2, sort_keys=True) + "\n").encode()
    for name, data in generated.items():
        (release / name).write_bytes(data)
    (release / "SHA256SUMS").write_text("".join(f"{sha256(data)}  {name}\n" for name, data in sorted(generated.items())))
    # Remove only prior, checksum-verified generator artifacts no longer selected.
    for path in existing:
        if path.name != "SHA256SUMS" and path.name not in generated:
            path.unlink()
    return source, release


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("version", nargs="?", default=(ROOT / "VERSION").read_text().strip())
    parser.add_argument("--output", type=Path, default=ROOT / "publish")
    args = parser.parse_args()
    try:
        source, release = prepare(args.version, args.output)
    except ValueError as error:
        raise SystemExit(str(error))
    print(f"GitHub source: {source}")
    print(f"Release assets: {release}")
    print("No repository, commit, tag, push, or GitHub release was created")


if __name__ == "__main__":
    main()
