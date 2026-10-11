# Contributing

Use Go 1.26.7 and Python 3.12 or later for the documented release tools.
Python, Go, and build utilities are not end-user runtime requirements.

```sh
go mod download
go test ./...
go vet ./...
go test -race ./...
python3 -m unittest discover -s tools/tests
python3 tools/verify-public.py
```

Keep credentials, live console tokens, cookie values, private keys, and lab
inventories out of issues, fixtures, logs, and commits. Fixtures must identify
whether they are sanitized observed responses or synthetic test cases. Real
infrastructure is not needed for the automated suite.

Bundled noVNC is pinned and unmodified. To regenerate assets/notices:

```sh
python3 tools/vendor-assets.py
```

Review the resulting diff, preserved headers, licenses, source checksum, and
dependency versions. Do not casually reformat vendored third-party files.
Changes to MPL-covered files retain MPL-2.0 and require matching source delivery.
See `THIRD_PARTY_NOTICES.md`.

Prepare release artifacts with `python3 tools/prepare-publication.py 0.1.0`.
Publishing instructions and directory layout are in `docs/publishing.md`.

Contributions to original project files are provided under the project MIT
license. Third-party files retain their existing license terms.
