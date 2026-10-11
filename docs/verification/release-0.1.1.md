# Release 0.1.1 qualification

Prepared from `feature/vm-power-media-controls` on 2026-10-11. Version 0.1.1 adds
power and CD-ROM controls, powered-off viewer startup, independent management
and console lifecycles, protected asynchronous operations, and simplified media
labels. Original third-party dependencies/notices and supported targets remain
unchanged.

## Evidence and compatibility

The feature's live PC 7.6 results, synthetic browser checks, API contracts and
sanitized response projections are recorded in
[VM power/media verification](vm-power-media-controls.md) and
[PC compatibility](../pc-7.6-compatibility.md). Live observations used the current
feature implementation before its version metadata was bumped to 0.1.1.

Power off, off-state ISO insertion, explicit startup with media preserved, reset,
eject without drive deletion, and automatic/explicit console recovery succeeded
on the designated Ubuntu desktop VM. Both guest requests were accepted but their
tasks failed with `VMM-10000` / `INTERNAL_ERROR`; no guest-tools configuration was
reported, and the exact cause was not established. Successful guest OS reboot or
shutdown is not qualified; failures remained visible with no hard fallback.

Pagination errors, permission denial, conflict serialization, timeout/cancellation,
local endpoint protection, and hidden-page/stale-callback behavior are covered
by synthetic tests, not claimed as additional live observations.

## Platform scope

Supported packages: Windows amd64, macOS arm64, Linux amd64 and Linux arm64.
Intel macOS remains excluded. Historical native interactive qualification is in
[the 0.1.0 record](release-0.1.0.md); it does not imply new 0.1.1 interactive
qualification on every platform. The local release checks execute the exact
packaged Linux amd64 binary offline. The tag-triggered GitHub workflow performs
native offline checks on all four supported targets before publishing; its
future results are not claimed as completed local observations.

## Release preparation checks

Executed local checks passed:

- `go test ./...`, `go test -race ./...`, and `go vet ./...`.
- All six publication-tool tests and public-source/notices checks for 145
  allowlisted files, with credentials/private workflow material excluded.
- Four CGO-disabled 0.1.1 builds, complete archive/source/checksum verification,
  and native offline startup of the exact packaged Linux amd64 executable.
- The manifest's qualification record points to `docs/verification/release-0.1.1.md`.

The packaged Linux amd64 binary SHA-256 is
`b89ddaf51c1a5229ea7b6022459e48ecc9a8b31c16b22be3391a033529c1cc25`.
The macOS arm64 binary SHA-256 is
`3280d6eca4e90e9f2201665a3427edea57fbb4a018ddf7f5ad0cc0fe8f3dcafc`.
Archive hashes and compiler/source records are in `SHA256SUMS` and
`release-manifest.json`. Local packages are staged under
`publish/v0.1.1-staging/releases/v0.1.1/`. GitHub's tag-triggered checks and
publication are separate from these local preparation results.
