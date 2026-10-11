# Publishing the public repository and release

## Prepare locally

Run from the source workspace with Go 1.26.7 and Python 3.12+:

```sh
go mod download
go test ./...
go vet ./...
go test -race ./...
python3 -m unittest discover -s tools/tests
python3 tools/verify-public.py
python3 tools/prepare-publication.py 0.1.0
python3 tools/verify-release.py publish/releases/v0.1.0
```

```text
publish/
├── github/                       # Clean source tree to push to GitHub
│   ├── .github/workflows/        # Four-target native CI and tag release workflow
│   ├── cmd/ahv-console/
│   ├── internal/                 # Go implementation, embedded assets/notices
│   ├── testdata/                 # Sanitized observed and labeled synthetic fixtures
│   ├── tools/                    # Reproducible build/export/verification tools
│   ├── docs/                     # Public compatibility, qualification, publishing docs
│   ├── LICENSE                   # MIT, Copyright (c) 2026 Kurt Telep
│   ├── THIRD_PARTY_NOTICES.md
│   ├── VERSION                   # 0.1.0
│   └── README.md
├── export-manifest.json          # Local export ownership/hash record; not repo source
└── releases/v0.1.0/              # Assets for GitHub Releases
    ├── ahv-console_0.1.0_windows_amd64.zip
    ├── ahv-console_0.1.0_darwin_arm64.tar.gz
    ├── ahv-console_0.1.0_linux_amd64.tar.gz
    ├── ahv-console_0.1.0_linux_arm64.tar.gz
    ├── ahv-console_0.1.0_source.tar.gz
    ├── novnc-1.6.0-source.tar.gz
    ├── release-manifest.json
    ├── RELEASE_NOTES.md
    └── SHA256SUMS
```

Every platform archive contains the executable, README, project MIT license,
third-party notice index, full `licenses/` texts, and matching noVNC source under
`source/`. Those accompanying files are redistribution materials, not application
runtime dependencies. See `THIRD_PARTY_NOTICES.md`.

The explicit export allowlist preserves public source, fixtures, licenses, and
documentation. It excludes the existing workspace's `.git` history, `.opencode`,
OpenSpec planning files, private lab inventories, screenshots, logs, and raw
build outputs. Push **`publish/github/`**, not the original development workspace.
The preparer refuses to overwrite an export that has local edits or has been
initialized as a Git repository; use a new `--output` directory if regenerating.
Do not copy older raw build outputs into the supported release directory.

## Create and push your GitHub repository

The preparation tool does not create a repository, commit, tag, push, or release.
After reviewing the generated tree, execute these yourself with your GitHub
account. Replace `OWNER` with your GitHub username/organization:

```sh
cd publish/github
git init -b main
git add .
git commit -m "Initial public release"
gh auth login
gh repo create OWNER/ahv-console --public --source=. --remote=origin --push
```

CI runs tests, notice/source checks, and offline native startup on Linux amd64,
Linux arm64, Windows amd64, and macOS arm64. These are the documented public
GitHub-hosted runner labels; runner logs identify the actual environments.
No Prism credentials or live infrastructure are used by CI.

## Publish version 0.1.0

After checking CI results, create and push the version tag:

```sh
git tag v0.1.0
git push origin v0.1.0
```

The tag-triggered release workflow checks that the tag matches `VERSION`, runs
the automated checks, builds and verifies bundles, then executes the exact
packaged binaries' offline smoke checks on all four native runner targets. It
creates the GitHub release with those same checked assets only after these jobs
succeed, using the repository's built-in workflow token.
GitHub-built hashes can differ from local hashes if the compiler distribution
differs; each published set has its own matching `SHA256SUMS` and compiler record.
The repository source at the tag is the source for that workflow-built release.

If publishing the locally prepared artifacts instead, omit the tag-triggered
workflow from your repository before pushing the tag, then upload from the
workspace's `publish/releases/v0.1.0/` directory:

```sh
gh release create v0.1.0 --repo OWNER/ahv-console --verify-tag \
  --title "AHV Console v0.1.0" --notes-file RELEASE_NOTES.md \
  ./*.zip ./*.tar.gz release-manifest.json RELEASE_NOTES.md SHA256SUMS
```

Preserve the platform notices/source and the separate noVNC source asset.
No code-signing certificates are assumed or generated. Optional future signing
requires a new verified checksum manifest for the signed artifacts.

## Future changes

Update `VERSION`, `CHANGELOG.md`, qualification evidence, and publication paths
together for later releases. Keep third-party versions/notices pinned and
regenerate them deliberately. macOS amd64 must be qualified before adding it to
supported `TARGETS`; an experimental cross-build is not qualification evidence.
