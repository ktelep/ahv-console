# PC 7.6 contract fixtures

Captured from the designated PC 7.6 test endpoint on 2026-10-10.

- `generate-console-token.accepted.json`: projection of the observed HTTP 202
  response containing its envelope type and task reference. Uninspected metadata
  is omitted. The task UUID is replaced with a synthetic UUID; the observed
  task identifier prefix is preserved.
- `task.succeeded.json`: projection of the observed HTTP 200 task response
  retaining its envelope type, task type, status, progress, operation, and
  completion-details entries. Unrelated metadata, ownership, timestamps, and
  affected-entity information are omitted. The task and VM identifiers are
  synthetic and the token value is explicitly redacted.
- `authentication.failed.json`: observed HTTP 401 body from a single request
  with deliberately invalid credentials for a synthetic nonexistent username.
  The endpoint returned JSON despite `Content-Type: text/plain`. No real user's
  password was deliberately submitted incorrectly.
- `list-vms.empty.observed.json`: projection of an observed HTTP 200 empty
  filtered collection. PC 7.6 omitted `data` and reported an explicit zero total
  with `hasError: false`; this is a normal no-match result, not a malformed list.

These files contain response bodies, not request credentials or headers.
The success files are explicitly reduced projections, not full raw responses.
They must not be represented as evidence of negative outcomes, pagination,
timeouts, or retry behavior. Fixture review found no password, cookie value,
or live token in these files. The placeholder token is not a valid JWT.

- `permission-denied.synthetic.json`: schema-based HTTP 403 error-response
  test double. The error code and message are invented test markers, not PC
  observations.
- `task-failed.synthetic.json`: schema-based failed-task test double. `FAILED`
  and `errorMessages` follow the official task schema; the error code and message
  are invented test markers, not PC observations.

- `task.permission-denied.observed.json`: reduced projection of a live HTTP 200
  failed console-generation task submitted by a newly provisioned Prism Viewer
  identity. The POST was accepted with HTTP 202, then the task failed with
  `VMM-30500` / `VM_UNAUTHORIZED_REQUEST`. The VM UUID is synthetic. This single
  observed response provides both permission-denial and failed-console-task
  evidence; it is not an observed HTTP 403 or a non-permission task failure.

Synthetic fixtures remain useful for generic task failures and immediate HTTP
403 handling, neither of which should be confused with the observed asynchronous
permission-denial behavior. No passwords, cookie values, or live tokens are
present in the observed negative fixture.

## Power/media observations (2026-10-11)

After explicit user approval, the designated PC 7.6 Ubuntu test VM and ISO were
used for live viewer verification. See
`docs/verification/vm-power-media-controls.md` for results and limits.

- `management.insert-task.observed.json`: reduced projection of the observed
  successful insert task, independently retrieved from the selected VM's PC
  v4.4 task inventory after the viewer completed mounting.
- `management.guest-reboot-task.observed.json` and
  `management.guest-shutdown-task.observed.json`: reduced projections of observed
  failed guest requests, retaining actual operation, status, error code and group.
  Both returned `VMM-10000` / `INTERNAL_ERROR`; neither is guest-success evidence.
- `management.mounted-drive.observed.json`: reduced drive projection after mount
  while the VM remained Off, retaining IDE 1, backing size and image reference.
  The collection metadata and unrelated backing/storage properties are omitted.

Task/drive/image UUIDs are replaced with synthetic UUIDs. The task prefix,
operation names, statuses, size, discriminator, error codes and groups are
observed values. Unrelated task data, messages, argument maps, endpoint addresses,
ownership and timestamps are omitted. These projections are not raw captures
and do not prove guest prerequisites, retry behavior, or role coverage.
