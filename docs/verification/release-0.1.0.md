# Release 0.1.0 qualification

## Supported initial targets

The initial supported release is Windows amd64, macOS arm64, and Linux
amd64/arm64. The maintainer approved excluding unqualified macOS amd64 from
supported artifacts. Experimental Intel Mac builds remain separate.

| Target | Environment | Evidence | Confirmed behavior |
| --- | --- | --- | --- |
| Windows amd64 | Windows Server 2019 | User reported | Startup, automatic browser, display, keyboard/mouse, refresh, Ctrl+C |
| macOS arm64 | macOS 26.5, Apple Silicon | User reported | Startup, automatic browser, display, keyboard/pointer, refresh |
| Linux amd64 | Fresh Ubuntu Desktop 24.04.5, kernel 6.8.0-142-generic, Chrome 155.0.8059.39 | Directly verified | Startup, offline commands, automatic native browser, display, keyboard/pointer, refresh, Ctrl+C, listener cleanup |
| Linux arm64 | ARM64 Ubuntu Desktop VM on Apple Silicon; `uname -m` reported `aarch64` | User reported | Startup, automatic browser, display, keyboard/mouse, refresh, Ctrl+C |

The Windows/macOS/Linux arm64 checks concern development builds of the same
application implementation. They are historical interactive evidence, not
claims that every final archive was rerun manually on every machine. Native
offline smoke checks for versioned source are provided by
`tools/native-smoke.py` and the four-target GitHub CI matrix; CI outcomes only
exist after those workflows actually run. Browser versions and Ubuntu ARM guest
version were not supplied for the user-reported tests. macOS Ctrl+C was not
explicitly reported. No earlier-OS-baseline or Intel Mac result is inferred.

The fresh Ubuntu amd64 VM ran only a copied executable, with no app source tree,
external viewer assets, Go or Node.js installation, or separately installed VNC
proxy. The ELF binary is statically linked. Help/version/license commands worked
with an empty environment and a nonexistent PATH. Temporary browser inspection
tools were removed after testing; those tools are not application dependencies.

## PC/API and negative-path evidence

The designated test PC reported PC 7.6. Authentication, VM lookup, console-token
submission, task completion, authenticated binary WSS, and an RFB 3.8 handshake
succeeded without an existing Prism browser session. The compatibility table
and sanitized response fixtures are in `docs/pc-7.6-compatibility.md` and
`testdata/pc76/`. No real endpoint, username, VM UUID, cookie value, password,
or console JWT is included in this public record.

Live duplicate-name fixtures confirmed candidate listing, unattended ambiguity,
cluster filtering, invalid selection rejection, explicit selection, and UUID
bypass. Default TLS rejected the test endpoint's self-signed certificate.
An empty filtered VM lookup returned HTTP 200 with no `data` field and an explicit
zero total. The client recognizes this as a lookup miss, with a regression test
that still rejects missing-data responses without an explicit zero count.
Simulated browser-launch failure left a working viewer available through its
printed URL. Viewer assets loaded with non-loopback asset requests blocked.

A read-only Prism Viewer identity could read the VM and cluster but received
HTTP 202 followed by a failed console-generation task carrying `VMM-30500` /
`VM_UNAUTHORIZED_REQUEST`. The CLI recognizes that asynchronous permission
failure, exits 4, and starts no viewer. The observed fixture has synthetic
identifiers and no live secrets. Generic task failures and immediate HTTP 403
responses remain clearly labeled synthetic cases.

## Release checks

The documented preparation sequence verifies:

- `go test ./...`, `go vet ./...`, and Linux `go test -race ./...`.
- Publication-policy and deterministic-archive unit tests.
- Four CGO-disabled versioned binaries and offline native smoke checks.
- Embedded original-code, noVNC, pako, DES, WebSocket, and Go license notices.
- Exact noVNC source checksum and equality of runtime vendored files to source.
- Platform/source archive contents and all checksums.
- Clean public-source allowlist excluding private lab records, credentials,
  local workspace paths, agent configuration, and planning artifacts.

Generated `release-manifest.json` identifies the compiler, artifact hashes,
source-file hashes, supported/excluded targets, and noVNC provenance. Locally
executed final checks are recorded below when preparation completes; workflow
definitions are not themselves evidence that GitHub jobs passed.

### Executed local final checks

Preparation of version 0.1.0 passed:

- Go tests, race tests, and vet on the development Linux host.
- The publication-policy/deterministic-archive unit suite and public-source
  privacy/notice/source checks.
- Four supported CGO-disabled builds and full release-bundle verification.
- Independent build/tests/vet from the generated public source export.
- Native execution of the exact packaged Linux amd64 binary: offline help,
  version, and complete license notices.
- Startup/offline commands on the fresh Ubuntu guest with no executable PATH
  dependencies, using the same versioned binary and matching checksum.
- Live versioned-binary lookup miss: exit 5, no viewer, no credential output.
- Live versioned-binary console rendering and refresh in an isolated browser,
  with non-loopback asset requests blocked; interruption closed the listener.
- GitHub workflow syntax checked with actionlint v1.7.7.

The locally prepared Linux amd64 executable SHA-256 is
`f88d2742e19c213015b48faf5331fcc5d8e6d90e3fb9e182bf29adfadef7709e`.
All platform/source archive hashes are generated in `SHA256SUMS` and
`release-manifest.json`. No GitHub repository, commit, tag, push, or release was
created during preparation. Tag-workflow native smoke checks on GitHub have
not yet run and are not claimed as completed local checks.
