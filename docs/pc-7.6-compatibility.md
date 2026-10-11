# PC 7.6 connection contract — live handshake verified

## Status

Research date: 2026-10-10. The designated endpoint reports `pc.7.6`. Its
self-signed certificate fails system-trust verification; the user explicitly
authorized disabling verification for this endpoint during the test.
Username/password REST authentication, exact-name VM lookup, console-token
generation, task retrieval, cookie-authenticated WSS, and the RFB handshake
have now succeeded without a pre-existing Prism browser login.

The successful connection and asynchronous permission-failure contracts have
been observed, with sanitized success and failure fixtures retained;
this is not release qualification. See `verification/pc-7.6-handshake.json`
for sanitized observations. No password, cookie value, or console token is
included in this record.

## Sources and evidence limits

- [Nutanix console-access article, May 1, 2026](https://www.nutanix.dev/2026/05/01/vm-console-external-access/):
  read directly; describes token generation, user authentication on WSS, and
  console permissions, but contains inconsistent examples.
- [Official VMM v4.3 reference](https://developers.nutanix.com/api-reference?namespace=vmm&version=v4.3)
  and [Prism v4.4 reference](https://developers.nutanix.com/api-reference?namespace=prism&version=v4.4):
  direct retrieval initially returned a JavaScript application shell. Subsequent
  browser inspection succeeded, including the console-token operation's
  authentication, request headers, 202 response schema, and supported versions.
- Official OpenAPI documents inspected for request and response contracts:
  [VMM v4.3](https://developers.nutanix.com/api/v1/namespaces/vmm/versions/v4.3/yaml),
  [Prism v4.4](https://developers.nutanix.com/api/v1/namespaces/prism/versions/v4.4/yaml),
  and [Cluster Management v4.3](https://developers.nutanix.com/api/v1/namespaces/clustermgmt/versions/v4.3/yaml).
  These are the documents served by the official reference application.
- Available Nutanix API tool schemas were inspected for `ahv_listVms`,
  `ahv_getVmById`, `generateConsoleTokenById`, and `getTaskById`.
  These establish request-side metadata only; their returned schemas do not
  include response models or prove availability on a specific PC 7.6 build.

## Operation compatibility table

Paths below include the `/api` deployment prefix used by the article; the tool
schemas report paths starting at the namespace.

| Operation | Version and request from tool schema | Request details | Response / PC 7.6 observation |
| --- | --- | --- | --- |
| List AHV VMs | v4.3 `GET /api/vmm/v4.3/ahv/config/vms` | `$page` starts at 0; `$limit` 1–100, default 50; OData `$filter` supports `name` and `cluster/extId` | Observed HTTP 200, `data` array, `metadata.totalAvailableResults: 1`; exact-name filter matched one powered-on test VM; multi-page behavior not yet exercised |
| Read AHV VM | v4.3 `GET /api/vmm/v4.3/ahv/config/vms/{extId}` | VM UUID path parameter; no request body | Observed HTTP 200; `data` is `vmm.v4.ahv.config.Vm`, with matching `extId` and a `name` string |
| Generate console token | v4.3 `POST /api/vmm/v4.3/ahv/config/vms/{extId}/$actions/generate-console-token` | No request body; required UUID `NTNX-Request-Id` used for idempotence | Observed HTTP 202, `vmm.v4.ahv.config.GenerateConsoleTokenApiResponse`, `data` is `prism.v4.config.TaskReference` with `extId` |
| Read task | v4.4 `GET /api/prism/v4.4/config/tasks/{extId}` | Preserve full prefixed task ID and URL-encode as a path segment; no request body | Observed HTTP 200, `data.status: SUCCEEDED`; `data.completionDetails` is an array of `{name, value}` entries named `VmConsoleToken` and `WsUri`, with string values |
| Resolve cluster names | v4.3 `GET /api/clustermgmt/v4.3/config/clusters/{extId}` | Use VM `cluster.extId`; no request body | Observed HTTP 200; `data` is `clustermgmt.v4.config.Cluster`, with matching `extId` and a `name` string |
| Identify PC version | v4.4 `GET /api/prism/v4.4/config/domain-managers` | HTTP Basic authentication | Observed HTTP 200; `data[0].config.buildInfo.version` is `pc.7.6` |
| Establish user session | Authenticated REST request | HTTP Basic username/password over HTTPS | Observed `NTNX_SESSION_ID` cookie, path `/`, Secure, HttpOnly, SameSite=Lax; REST-issued cookie alone authenticated the tested WSS connection |
| Connect console | Returned relative URI resolved to `wss://<PC>:9440/console/launch/<VM_UUID>?VmConsoleToken=<token>` | REST-issued cookie and console token; tested Origin equal to configured PC HTTPS origin | Observed binary transport, empty negotiated subprotocol, complete RFB 3.8 handshake; security type 1, successful security result, shared ClientInit, ServerInit at 1024×768 and 32 bits/pixel |

The standalone client must generate its own request UUID and preserve it when
retrying the same logical submission. Tool-side header auto-injection is not a
feature of a standalone HTTP client.

Empty filtered VM collections were observed returning HTTP 200 with no `data`
field, `metadata.totalAvailableResults: 0`, and `hasError: false`. Treat an
explicit zero total with missing data as no match; missing data without that
empty-result contract remains a malformed-response compatibility failure.

### Authentication, headers, and polling contract

The official definitions declare HTTP Basic authentication (`type: http`,
`scheme: basic`); API-key authentication is also documented but outside this
change. The standalone client uses Basic authentication over HTTPS, accepts
JSON responses, and retains the REST-issued cookie in a cookie jar. Attach
the cookie to the configured PC WSS origin together with the console token
query parameter. The live test used the configured PC HTTPS Origin, no offered
subprotocol, and binary WebSocket messages. An absent or different Origin and
alternative subprotocols have not been qualified.

Generate-console-token is a bodyless POST with a required `NTNX-Request-Id`.
The official 202 schema includes `data.extId` as a task reference and documents
a `Location` response header. The tested workflow follows `data.extId` using
the v4.4 task GET; it does not depend on `Location`. The reference lists PC 7.5
and PE 7.5 as minimum supported versions for this v4.3 operation; our actual
observations establish success specifically on the designated PC 7.6 system.

The official `prism.v4.4.config.TaskStatus` enum contains `QUEUED`, `RUNNING`,
`CANCELING`, `SUCCEEDED`, `FAILED`, `CANCELED`, `SUSPENDED`, `$UNKNOWN`, and
`$REDACTED`. Poll the task GET, not the token POST. `SUCCEEDED` permits result
parsing; `FAILED` and `CANCELED` are failure outcomes. Pending/suspended states
must remain bounded by the setup deadline; unknown/redacted states must not be
treated as success. The deadline, cancellation, and backoff are client policies,
not claimed server behavior. `SUCCEEDED` and `FAILED` have been observed live.

The official task model defines `completionDetails` as a list of
`common.v1.0.config.KVPair`, whose `value` is a union. For this operation, live
values for `VmConsoleToken` and `WsUri` were strings. Require both names and
string values; do not assume a generic KVPair value is always a string.
`errorMessages` is a list of `prism.v4.4.error.AppMessage`, documented as populated
when a task fails. A restricted Prism Viewer identity received HTTP 202 for
console-token submission, followed by `FAILED` with `VMM-30500` and
`errorGroup: VM_UNAUTHORIZED_REQUEST`. This is asynchronous permission denial,
not an HTTP 403. The CLI classifies these known task markers as permission
failures without displaying upstream messages or argument maps. Other failed
tasks remain task failures. See `task.permission-denied.observed.json`.

A request with deliberately invalid credentials for a synthetic nonexistent
user returned HTTP 401, `Content-Type: text/plain`, with a JSON error envelope
containing `api_version: 3.1` and reason `AUTHENTICATION_REQUIRED`. This is an
observed gateway response on the v4.3 request, not a legacy API fallback. A
request with no authentication returned HTTP 302; redirects were disabled in
the probe. Clients must handle status before assuming a v4 response shape.

## Article inconsistencies and live resolution

1. The introduction links VMM v4.2+, while the POST example uses v4.0. Neither
   establishes PC 7.6 support by itself. The operation combination above was
   subsequently observed on the designated PC 7.6 endpoint.
2. The article says to poll the generation API. The project design instead
   requires following the returned task. Live verification used the v4.4 task
   GET endpoint successfully. Do not repeatedly POST to emulate polling.
3. Prose names `console_websocket_uri` and `console_token`; the JSON example
   names `WsUri` and `VmConsoleToken`. Their nesting in task completion details
   is now observed as `data.completionDetails` name/value entries using the
   latter names. Parser tests must use sanitized fixtures from those responses.
4. REST basic authentication does not prove that WSS accepts basic
   authentication or that REST returns a reusable session cookie. The article
   does not supply the session-establishment flow. The live test established
   that REST authentication issues a cookie accepted by the WSS endpoint.

The article identifies `Generate_VM_Console_Token` as a required permission.
The supplied account successfully performed lookup, task-read, and console
operations. The newly provisioned Prism Viewer test identity could read the VM
and cluster, but console generation failed asynchronously as described above.
The identity was disabled after verification. Other role combinations remain
unqualified.

## Evidence limits

### Power and CD-ROM schema contract (2026-10-11)

The official VMM v4.3 and Prism v4.4 OpenAPI documents linked above were
rechecked for these operations. The table is **schema evidence**. Separate
controlled PC 7.6 live observations are recorded below and in
`verification/vm-power-media-controls.md`; they do not qualify every role,
guest OS, PC build, or image placement.

| Operation | Exact deployment path | Body and success contract |
| --- | --- | --- |
| VM state | GET `/api/vmm/v4.3/ahv/config/vms/{extId}` | HTTP 200 `data` VM; `powerState`: `ON`, `OFF`, `PAUSED`, `UNDETERMINED`, `$UNKNOWN`, `$REDACTED` |
| Power on | POST `/api/vmm/v4.3/ahv/config/vms/{extId}/$actions/power-on` | No body; HTTP 202 task reference |
| Power off | POST `/api/vmm/v4.3/ahv/config/vms/{extId}/$actions/power-off` | No body; HTTP 202 task reference; forced shutdown |
| Restart (hard) | POST `/api/vmm/v4.3/ahv/config/vms/{extId}/$actions/reset` | No body; HTTP 202 task reference; not power-cycle |
| Restart guest OS | POST `/api/vmm/v4.3/ahv/config/vms/{extId}/$actions/guest-reboot` | Required `GuestPowerOptions` body; minimal schema-valid body `{}`; HTTP 202 task reference |
| Shut down guest OS | POST `/api/vmm/v4.3/ahv/config/vms/{extId}/$actions/guest-shutdown` | Required `GuestPowerOptions` body; minimal schema-valid body `{}`; HTTP 202 task reference |
| CD-ROM list/detail | GET `/api/vmm/v4.3/ahv/config/vms/{vmExtId}/cd-roms` and `/{extId}` | HTTP 200 `data` array/object of `CdRom` |
| Mount ISO | POST `/api/vmm/v4.3/ahv/config/vms/{vmExtId}/cd-roms/{extId}/$actions/insert` | Required `CdRomInsertParams`; HTTP 202 task reference |
| Eject | POST `/api/vmm/v4.3/ahv/config/vms/{vmExtId}/cd-roms/{extId}/$actions/eject` | No body; HTTP 202 task reference; never DELETE |
| Images | GET `/api/vmm/v4.3/content/images` and `/{extId}` | HTTP 200 `data` array/object of `Image`; ISO enum is `ISO_IMAGE`, disk enum is `DISK_IMAGE` |
| Task | GET `/api/prism/v4.4/config/tasks/{extId}` | HTTP 200 `data` Task; retain and path-encode complete prefixed task ID |

All listed mutations require a UUID `NTNX-Request-Id`; preserve it across
same-submission retries. The 202 `data.extId` is a task reference (wire type
`prism.v4.config.TaskReference`), not completion. Task statuses and error fields
are described above; no new live task outcomes are implied by this inspection.

Guest requests use NGT according to the official operation descriptions.
`GuestPowerOptions.guestPowerStateTransitionConfig` is optional, and its
`shouldEnableScriptExec` and `shouldFailOnScriptFailure` fields are optional.
The minimal body does not opt into scripts. NGT installation/readiness and
guest policy behavior are not qualified by the failed guest requests observed
below; failures must not trigger a hard-action fallback.

Insertion body:

```json
{"backingInfo":{"dataSource":{"reference":{"$objectType":"vmm.v4.ahv.config.ImageReference","imageExtId":"<image UUID>"}}}}
```

`ImageReference.imageExtId` is required; `storageCluster` is optional and
describes intended VM storage placement. Do not add it to imply image migration.
`CdRom` has `extId`, `diskAddress.busType` (`IDE`/`SATA`), `diskAddress.index`,
and optional `backingInfo` (`VmDisk`). Backing information includes `diskExtId`
and optional `dataSource.reference`; unresolved backing must be shown as mounted
media rather than empty.

Both CD-ROM and image collections support zero-based `$page` and `$limit`
1–100 (default 50). Image type filtering is documented as an OData enum literal
`type eq Vmm.Content.ImageType'ISO_IMAGE'`; also check types locally.
`metadata.totalAvailableResults` supplies the collection total. A failed or
repeated page must not be presented as a complete inventory.

Image `clusterLocationExtIds` describes where an image is located; it is not
a guarantee of insertion success. Missing/redacted placement remains unknown.
`placementPolicyStatus` describes policy compliance/enforcement, not a generic
image-ready flag: entries include `complianceStatus`, `enforcementMode`,
`policyClusterExtIds`, and `enforcedClusterExtIds`. Refresh image detail when
list placement is insufficient. Show target-cluster location evidence, label
known nonlocal placement as unavailable locally, and leave uncertain placement
explicit. PC remains authoritative for insertion; no migration is initiated.

Permissions are operation-specific: View Virtual Machine, View Virtual Machine
CD ROM, View Image, Power On/Off Virtual Machine, Reset Virtual Machine, Guest
Reboot/Shutdown Virtual Machine, Insert/Eject Virtual Machine CD ROM, and task
reads. Visibility or console permission alone does not establish these grants.

### Controlled power/media live observations (2026-10-11)

With explicit user approval, the designated PC reported `pc.7.6`; the existing
Ubuntu desktop VM was tested with the designated AHV DVD ISO on IDE 1. Power off,
off-state insert, explicit power on, reset, and eject tasks succeeded. State read
after insertion remained `OFF`; the drive's `dataSource.reference` reported
`vmm.v4.ahv.config.ImageReference` and the selected image UUID. Power on preserved
the media and triggered successful automatic console acquisition/RFB connection.
After reset, Reconnect succeeded using the existing accepted authorization. Eject
removed backing without deleting either drive. Final state was On, IDE 1 empty,
and IDE 0's original customization media unchanged.

Both minimal-body guest requests were accepted as asynchronous tasks, then
`GuestRebootVm` and `GuestShutdownVm` failed with `VMM-10000` /
`INTERNAL_ERROR`. The VM read reported no guest-tools configuration; the generic
error does not establish the precise failure cause. The viewer displayed each
failure and the VM remained On, with no hard fallback. Successful guest OS
transitions remain unqualified. No guest tools/policies were changed.

The tested ISO was `ISO_IMAGE`, with 944766976-byte size and target-cluster
membership in `clusterLocationExtIds`. This verifies one local-placement success,
not nonlocal/unknown placement or automatic transfer. Pagination, permission
denial, timeout/cancellation, and protection cases remain synthetic coverage.
See the feature verification record and `testdata/pc76/management.*.observed.json`
for sanitized observed projections, separately labeled from synthetic fixtures.

Success, invalid-authentication, and failed-console-task permission denial
fixtures are captured in `testdata/pc76`. The permission denial and failed-task
evidence come from the same observed asynchronous response; they are not two
different server failure mechanisms. Synthetic HTTP 403 and generic task-failure
fixtures remain labeled as synthetic. Native platform qualification is recorded
separately in `verification/implementation.md`.

The successful probe used fresh isolated contexts: REST Basic authentication
issued the cookie, and a separate context with no HTTP credentials used only
that cookie for WSS. A locally fulfilled empty page supplied the configured PC
Origin without loading the Prism UI. Chromium's local-network permission was
needed for this test harness; the initial blocked attempt never reached WSS.
The probe exchanged the RFB version, security selection/result, and shared
ClientInit/ServerInit, then closed the connection without sending guest input.
No existing browser session or undocumented login endpoint was used.

Capture success, authentication failure, permission failure, and task failure fixtures without
passwords, cookie values, or live console tokens. Do not manufacture observed
fixtures or mark the tasks complete from documentation alone.

The username/password-to-WSS integration gate and live negative-fixture
follow-up under task 6.5 have passed.
