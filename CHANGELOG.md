# Changelog

## 0.1.0

Initial public release.

- Standalone PC 7.6 AHV console client with username/password authentication.
- Exact-name and UUID selection, complete pagination, cluster filtering, and
  explicit duplicate-name selection.
- Correct no-match diagnostics for PC 7.6's empty-list response with omitted data.
- Asynchronous console-token tasks, including observed asynchronous permission
  failure classification.
- Embedded noVNC, protected loopback viewer, native browser launch, refresh,
  foreground session ownership, and Ctrl+C cleanup.
- Default TLS verification, custom PEM CA support, and explicit upstream-only
  insecure mode.
- Four supported targets: Windows amd64, macOS arm64, Linux amd64/arm64.
- MIT license for original code, complete embedded third-party notices, exact
  noVNC source alongside binaries, source archives, and checksum manifests.

Intel macOS is not qualified and is excluded from supported release assets.
