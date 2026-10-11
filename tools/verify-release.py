#!/usr/bin/env python3
"""Verify checksums, supported targets, binary hashes, notices, and source bundles."""
import argparse
import io
import json
from pathlib import Path, PurePosixPath
import tarfile
import zipfile

from public_files import NOVNC_SHA256, REQUIRED_NOTICES, check_content, sha256

SUPPORTED = {("windows", "amd64"), ("darwin", "arm64"), ("linux", "amd64"), ("linux", "arm64")}


def unpack(path):
    entries = {}
    if path.suffix == ".zip":
        with zipfile.ZipFile(path) as archive:
            for member in archive.infolist():
                if not member.is_dir():
                    entries[member.filename] = archive.read(member)
    else:
        with tarfile.open(path, "r:gz") as archive:
            for member in archive:
                if not member.isfile():
                    raise ValueError("Unexpected non-file archive entry")
                entries[member.name] = archive.extractfile(member).read()
    for name in entries:
        path = PurePosixPath(name)
        if path.is_absolute() or ".." in path.parts:
            raise ValueError("Unsafe archive path")
    return entries


def resolve_release_directory(directory):
    """Accept a flat bundle or the single version folder from older uploads."""
    directory = Path(directory)
    if directory.is_symlink() or not directory.is_dir():
        raise ValueError(f"Release directory is missing or unsafe: {directory}")
    if (directory / "release-manifest.json").is_file():
        return directory
    children = list(directory.iterdir())
    candidates = [path for path in children if path.is_dir() and not path.is_symlink() and (path / "release-manifest.json").is_file()]
    if len(candidates) > 1:
        raise ValueError("Multiple release directories found; specify the intended version directory explicitly")
    if len(candidates) == 1:
        if len(children) != 1:
            raise ValueError("Unexpected sibling files in downloaded artifact; specify the release directory explicitly")
        return candidates[0]
    raise ValueError(f"No release-manifest.json at {directory} or in a single immediate child directory; check artifact upload/download paths")


def verify(release):
    release = resolve_release_directory(release)
    manifest = json.loads((release / "release-manifest.json").read_text())
    sums = {name: digest for digest, name in (line.split() for line in (release / "SHA256SUMS").read_text().splitlines())}
    actual = {path.name: sha256(path.read_bytes()) for path in release.iterdir() if path.is_file() and path.name != "SHA256SUMS"}
    if any(path.is_symlink() or not path.is_file() for path in release.iterdir()) or actual != sums:
        raise ValueError("Release files/checksums differ or include unexpected paths")
    targets = manifest["supportedTargets"]
    if len(targets) != 4 or {(x["os"], x["architecture"]) for x in targets} != SUPPORTED:
        raise ValueError("Supported release matrix is not the approved four targets")
    expected_assets = {x["archive"] for x in targets} | {manifest["sourceArchive"], "novnc-1.6.0-source.tar.gz", "RELEASE_NOTES.md", "release-manifest.json"}
    if set(actual) != expected_assets:
        raise ValueError("Release directory has unsupported or unexpected assets")
    if sha256((release / "novnc-1.6.0-source.tar.gz").read_bytes()) != NOVNC_SHA256:
        raise ValueError("Standalone noVNC source checksum mismatch")
    for target in targets:
        archive = release / target["archive"]
        if sha256(archive.read_bytes()) != target["archiveSha256"]:
            raise ValueError("Platform archive checksum mismatch")
        entries = unpack(archive)
        prefix = f"ahv-console_{manifest['version']}_{target['os']}_{target['architecture']}/"
        executable = "ahv-console.exe" if target["os"] == "windows" else "ahv-console"
        required = ["LICENSE", "README.md", "THIRD_PARTY_NOTICES.md", executable, "source/novnc-1.6.0-source.tar.gz"] + ["licenses/" + name for name in REQUIRED_NOTICES]
        if set(entries) != {prefix + name for name in required}:
            raise ValueError("Platform archive is missing required files or has unexpected files")
        if sha256(entries[prefix + executable]) != target["binarySha256"]:
            raise ValueError("Executable checksum mismatch")
        if sha256(entries[prefix + "source/novnc-1.6.0-source.tar.gz"]) != NOVNC_SHA256:
            raise ValueError("Bundled noVNC source checksum mismatch")
        for name, data in entries.items():
            if not name.endswith("novnc-1.6.0-source.tar.gz"):
                check_content(name, data)
    source_archive = release / manifest["sourceArchive"]
    if sha256(source_archive.read_bytes()) != manifest["sourceArchiveSha256"]:
        raise ValueError("Project source checksum mismatch")
    source = unpack(source_archive)
    prefix = "ahv-console-" + manifest["version"] + "/"
    source_hashes = {name.removeprefix(prefix): sha256(data) for name, data in source.items()}
    if source_hashes != manifest["publicSourceFiles"]:
        raise ValueError("Source archive does not match its allowlisted file manifest")
    for name, data in source.items():
        relative = name.removeprefix(prefix)
        if not name.startswith(prefix) or any(part in (".git", ".opencode", "openspec", "publish", "dist") for part in PurePosixPath(relative).parts):
            raise ValueError("Private/non-source path in source archive")
        if relative != "internal/viewer/assets/novnc/SOURCE.tar.gz":
            check_content(relative, data, vendored=relative.startswith("internal/viewer/assets/novnc/"))
    return manifest


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    args = parser.parse_args()
    try:
        manifest = verify(args.directory)
    except (ValueError, KeyError) as error:
        raise SystemExit(str(error))
    print(f"Release {manifest['version']} verified: four platform bundles, complete notices, exact noVNC source, allowlisted project source, and all checksums")


if __name__ == "__main__":
    main()
