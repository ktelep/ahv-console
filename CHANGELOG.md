# Changelog

## 0.1.1

- Add a state-aware Power menu: Power on, Power off, Restart (hard), Restart guest
  OS, and Shut down guest OS. Hard restart uses reset; guest failures never trigger
  a hard-action fallback.
- Add CD-ROM drive selection, searchable ISO inventory, independent Mount ISO
  and Eject actions, and cluster-location availability labels.
- Start the viewer for powered-off VMs before console-token acquisition and
  automatically connect after explicit startup reaches observed On state.
- Keep management available after console failures; track asynchronous actions,
  serialize conflicting mutations, and report unconfirmed outcomes without replay.
- Protect management endpoints with invocation cookies, exact Host/Origin checks,
  strict bounded JSON bodies, and selected-VM/device validation.
- Refresh state while visible, label stale state, and pause routine polling when
  hidden. Simplify media labels by hiding UUIDs and style Power like toolbar buttons.
- Record controlled PC 7.6 power/media verification and synthetic failure/protection
  coverage. Guest reboot/shutdown failed on the designated VM and successful guest
  transitions remain unqualified; see `docs/verification/release-0.1.1.md`.
- Preserve standalone embedded packaging and the existing four supported targets
  without adding runtime dependencies.

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
