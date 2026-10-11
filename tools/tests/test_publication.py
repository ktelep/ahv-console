import importlib.util
import io
from pathlib import Path
import sys
import tarfile
import tempfile
import unittest
import zipfile

TOOLS = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(TOOLS))
from public_files import PUBLIC_DIRS, PUBLIC_DOCS, ROOT_FILES, check_content, source_files

spec = importlib.util.spec_from_file_location("publication", TOOLS / "prepare-publication.py")
publication = importlib.util.module_from_spec(spec)
spec.loader.exec_module(publication)

verifier_spec = importlib.util.spec_from_file_location("release_verifier", TOOLS / "verify-release.py")
verifier = importlib.util.module_from_spec(verifier_spec)
verifier_spec.loader.exec_module(verifier)


class PublicPolicyTests(unittest.TestCase):
    def test_private_material_is_rejected_without_echoing_it(self):
        examples = [
            b"-----BEGIN " + b"PRIVATE KEY-----",
            b"eyJ" + b"A" * 16 + b"." + b"B" * 16 + b"." + b"C" * 16,
            b"ghp_" + b"x" * 36,
            b"192." + b"168." + b"1.42",
            b"/home/" + b"somebody/" + b"Workspace/private",
        ]
        for secret in examples:
            with self.subTest(kind=secret[:3]):
                with self.assertRaises(ValueError) as raised:
                    check_content("candidate.txt", secret)
                self.assertNotIn(secret.decode(), str(raised.exception))
        check_content("example.txt", b"pc.example.com; fixture-secret; REDACTED_CONSOLE_TOKEN")

    def test_source_allowlist_excludes_private_workflow_and_lab_files(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for name in ROOT_FILES + PUBLIC_DOCS:
                path = root / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text("public")
            for name in PUBLIC_DIRS:
                (root / name).mkdir(exist_ok=True)
            for name in (".opencode/config.json", "openspec/config.yaml", "docs/verification/test-resources.md", "docs/verification/implementation.md", "dist/old.exe"):
                path = root / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text("private")
            names = {path.relative_to(root).as_posix() for path in source_files(root)}
            self.assertIn("LICENSE", names)
            self.assertIn("docs/verification/release-0.1.0.md", names)
            self.assertNotIn("docs/verification/test-resources.md", names)
            self.assertFalse(any(name.startswith((".opencode/", "openspec/", "dist/")) for name in names))

    def test_export_does_not_overwrite_user_edits(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "root"
            root.mkdir()
            file = root / "LICENSE"
            file.write_text("original")
            output = Path(directory) / "output"
            output.mkdir()
            # Exercise the ownership/edit guard before full export verification.
            source = output / "github"
            source.mkdir()
            (source / "README.md").write_text("user work")
            with self.assertRaisesRegex(ValueError, "non-generated"):
                publication.export_source(root, output, [file])
            self.assertEqual((source / "README.md").read_text(), "user work")


class ArchiveTests(unittest.TestCase):
    def test_archives_are_repeatable_and_keep_executable_mode_and_notices(self):
        entries = [("app/ahv-console", b"executable", True), ("app/LICENSE", b"license", False), ("app/source/novnc.tar.gz", b"matching source", False)]
        tgz = publication.tar_bytes(entries)
        self.assertEqual(tgz, publication.tar_bytes(list(reversed(entries))))
        with tarfile.open(fileobj=io.BytesIO(tgz), mode="r:gz") as archive:
            self.assertEqual(archive.getmember("app/ahv-console").mode, 0o755)
            self.assertEqual(archive.extractfile("app/LICENSE").read(), b"license")
            self.assertEqual(archive.extractfile("app/source/novnc.tar.gz").read(), b"matching source")
        zipped = publication.zip_bytes(entries)
        self.assertEqual(zipped, publication.zip_bytes(list(reversed(entries))))
        with zipfile.ZipFile(io.BytesIO(zipped)) as archive:
            self.assertEqual(archive.read("app/LICENSE"), b"license")
            self.assertEqual(archive.getinfo("app/ahv-console").external_attr >> 16 & 0o777, 0o755)


class DownloadLayoutTests(unittest.TestCase):
    def test_flat_and_version_nested_artifacts(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            flat = root / "flat"
            flat.mkdir()
            (flat / "release-manifest.json").write_text("{}")
            self.assertEqual(verifier.resolve_release_directory(flat), flat)
            download = root / "release-assets"
            nested = download / "v0.1.0"
            nested.mkdir(parents=True)
            (nested / "release-manifest.json").write_text("{}")
            self.assertEqual(verifier.resolve_release_directory(download), nested)

    def test_missing_ambiguous_and_unexpected_layouts_fail_clearly(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with self.assertRaisesRegex(ValueError, "No release-manifest"):
                verifier.resolve_release_directory(root)
            with self.assertRaisesRegex(ValueError, "missing or unsafe"):
                verifier.resolve_release_directory(root / "missing")
            for version in ("v0.1.0", "v0.2.0"):
                child = root / version
                child.mkdir()
                (child / "release-manifest.json").write_text("{}")
            with self.assertRaisesRegex(ValueError, "Multiple release"):
                verifier.resolve_release_directory(root)
            (root / "v0.2.0" / "release-manifest.json").unlink()
            with self.assertRaisesRegex(ValueError, "Unexpected sibling"):
                verifier.resolve_release_directory(root)


if __name__ == "__main__":
    unittest.main()
