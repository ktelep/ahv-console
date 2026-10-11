# VM power/media implementation verification

Date: 2026-10-11. Feature branch: `feature/vm-power-media-controls`.
Existing-application baseline: `efb1964` (`Initial application baseline`),
created with explicit user authorization before feature edits.

## Evidence classification

The synthetic browser section uses local TLS HTTP servers and the actual embedded
viewer, selected-VM controller, Prism transport, and task parser. The separate
live section records observations from the user-designated PC 7.6 endpoint and
Ubuntu test VM after explicit approval of all tested actions. Credentials,
endpoint addresses, local authorization URLs, cookie values, and console tokens
are omitted. Official schema evidence is in `../pc-7.6-compatibility.md`.

## Synthetic browser observations

The opt-in fixture is `internal/viewer/browser_fixture_test.go`, launched with
`AHV_CONSOLE_BROWSER_FIXTURE=1 go test -v ./internal/viewer -run '^TestBrowserFixture$' -count=1`.
It is skipped in normal Go test runs and expires after five minutes.

- Initial power state was Off. Power on was enabled; other power actions and
  Ctrl+Alt+Del were disabled. No token generation was submitted.
- Opening CD-ROM by keyboard displayed two drives. The searchable ISO picker
  distinguished duplicate names by full UUID and size; the nonlocal image was
  disabled and labeled Unavailable on VM cluster.
- Selecting the ready ISO and Mount ISO produced action history `[insert]`,
  mounted the selected identifier, and left power Off.
- Explicit Power on produced `[insert, power-on, generate-console-token]`;
  media remained unchanged. The deliberately denied console request appeared
  as a permission diagnostic while power/media controls remained usable.
- A delayed operation-response fixture showed all power actions disabled while
  pending. Guest restart denial produced only `guest-reboot`; the subsequent
  explicit Restart (hard) submitted `reset`, with no power-cycle or fallback.
- Empty ISO inventory and denied ISO inventory had distinct messages. Both left
  permitted power and eject available. No-drive inventory disabled mount/eject.
- Eject showed Empty and retained both CD-ROM devices.
- A deliberately delayed console-diagnostic callback could not overwrite the
  newer powered-off placeholder after explicit Power off.
- Simulated hidden-page visibility paused routine polling for six seconds:
  upstream VM read count remained 54 before/after. Returning visible refreshed.
- A synthetic VM-state HTTP 503 marked the observed Off state stale and disabled
  all power mutations; subsequent successful refresh restored fresh Off state.

Final synthetic action sequence:
`insert, power-on, generate-console-token, guest-reboot, reset, eject, power-off`.
Expected browser network errors came from deliberately denied console/image
requests; no secret-bearing upstream payload was displayed.

## Automated checks

- `go test ./...`: passed, including existing console regressions and new
  management, selected-VM controller, endpoint protection, and CLI lifecycle tests.
- `go test -race ./...`: passed.
- `go vet ./...`: passed.
- `git diff --check`: passed for feature changes.
- The suite was rerun after adding regression coverage using the observed failed
  guest-task projections; both failures remained explicit with one guest mutation
  and no hard fallback.

## Packaging checks (initial feature build)

- `python3 tools/verify-public.py`: passed for 144 allowlisted source files,
  including feature evidence and new observed fixtures; notices and exact noVNC
  source integrity passed.
- `python3 -m unittest discover -s tools/tests -v`: all six tooling tests passed.
- The existing publication tooling exported a clean temporary source tree, then
  ran `tools/prepare-publication.py 0.1.0` there. Windows amd64, macOS arm64,
  Linux amd64 and Linux arm64 builds all succeeded with CGO disabled. Historical
  workspace build/release artifacts were preserved.
- `tools/verify-release.py` verified all four platform bundles, checksums,
  required notices, exact noVNC source and the allowlisted project source.
- `tools/smoke-release.py` executed the packaged Linux amd64 binary: offline
  commands passed without contacting Prism. Its SHA-256 was
  `9b2764d421647bc6ec7f3ef34c54eb075defa1a73b77efe22d5b7267a3a7f2e5`.
- The existing `//go:embed assets` includes the new HTML/JavaScript/CSS together
  with noVNC; viewer asset-serving tests passed. `go.mod` and `go.sum` are
  unchanged, and no runtime dependency was added.

Version metadata was 0.1.0 for that local packaging check; these were feature
verification artifacts, not a new published release or new native interactive
qualification on all four platforms. The active branch is
`feature/vm-power-media-controls`; at that stage only the authorized baseline
was committed. Subsequent 0.1.1 preparation and qualification are recorded in
`release-0.1.1.md`.

## UI polish verification (2026-10-11)

Following user feedback, media labels no longer display drive/image UUIDs. Drives
use bus/index; ISO choices show name, size and availability, with stable numbered
suffixes for duplicate names. Unresolved mounted media uses Mounted media.
Identifiers remain the internal option values used for the exact selected action.

The synthetic browser fixture verified IDE 0 / SATA 1 labels, numbered duplicate
ISO labels stable during search, and mounting the selected image while remaining
Off. No UUIDs appeared in the media labels. Power opened using Enter and matched
the other toolbar buttons' computed color, background, padding, border, radius,
font and height. These are synthetic UI observations, not additional live actions.

## Live PC 7.6 observations

The domain-manager read reported `pc.7.6`. The user-designated VM resolved as
`AHV-Console-Ubuntu-Desktop-amd64`; the ISO was
`AHV-DVD-x86_64-11.2.0.2-28.iso` (`ISO_IMAGE`, 944766976 bytes), with target
cluster membership in `clusterLocationExtIds`. The VM was initially On. Both
existing IDE drives initially contained guest-customization media. Testing used
IDE 1; IDE 0 was left unchanged. The VM read reported no `guestTools` configuration.

The current standalone executable was run with the designated credentials only
in process memory and upstream-only `--insecure` for this known self-signed lab
endpoint. Its embedded viewer was operated through Chromium/Playwright.
All seven power/media requests returned local HTTP 202 acceptance; upstream task
results were followed by the real Prism client and independently checked using
the PC v4.4 task inventory for this VM. Reduced observed response projections
are in `../../testdata/pc76/management.*.observed.json`.

| Explicit action | Observed upstream operation/status | Observed result |
| --- | --- | --- |
| Power off | `PowerOffVm` / `SUCCEEDED` | State Off; console closed; management remained available |
| Mount ISO while off | `InsertCdRom` / `SUCCEEDED` | State remained Off; IDE 1 had `isoType: OTHER`, matching image reference and 944766976-byte backing; IDE 0 unchanged |
| Power on | `PowerOnVm` / `SUCCEEDED` | State On; selected ISO preserved; automatic console acquisition and RFB connection succeeded |
| Restart (hard) | `ResetVm` / `SUCCEEDED` | State On; explicit Reconnect recovered the console using existing authorization |
| Restart guest OS | `GuestRebootVm` / `FAILED` | `VMM-10000`, `INTERNAL_ERROR`; failure shown, state remained On, no hard fallback |
| Shut down guest OS | `GuestShutdownVm` / `FAILED` | `VMM-10000`, `INTERNAL_ERROR`; failure shown, state remained On, no hard fallback |
| Eject | `EjectCdRom` / `SUCCEEDED` | IDE 1 backing absent, viewer showed Empty, both drives retained, state On and console connected |

Initial console connection, automatic post-startup connection, and explicit
post-reset recovery were observed Connected. PC task inventory contained exactly
the expected tested power/media operations and successful console generation,
with no intervening hard fallback for the failed guest requests. Typing into the
ISO search field while the console was connected retained focus on `iso-search`.
Ctrl+C stopped the foreground test executable and closed its listener/session.

Final VM state: **On**, IDE 1 **empty**, both drives retained, IDE 0's original
guest-customization media unchanged. No additional VM was deployed.

### Live evidence limits

Successful guest OS restart/shutdown is **not qualified**: both guest requests
failed, no guest-tools configuration was reported, and PC's generic internal
error does not establish the precise cause. No guest-tool installation or guest
policy changes were performed. Unknown/nonlocal placement, pagination failures,
permission-denied management, timeout/cancellation, and cross-session protection
are covered by synthetic tests rather than new live observations. Live success
applies to this designated PC 7.6 build, VM, ISO, and tested credentials; it is
not a guarantee for all PC builds, guest OSs, roles, or image placements.
