# AHV Console

Open a Prism Central 7.6 AHV VM console with one standalone executable and your
existing browser. noVNC and the WebSocket proxy are embedded; no Node.js,
Python, VNC client, external assets, or background service is needed at runtime.

## Install version 0.1.0

Download the matching archive and `SHA256SUMS` from this repository's GitHub
Releases page. The supported initial release has four targets:

| Platform | Archive | Interactive verification |
| --- | --- | --- |
| Windows x64 | `ahv-console_0.1.0_windows_amd64.zip` | Windows Server 2019, user reported |
| macOS Apple Silicon | `ahv-console_0.1.0_darwin_arm64.tar.gz` | macOS 26.5, user reported |
| Linux x64 | `ahv-console_0.1.0_linux_amd64.tar.gz` | Fresh Ubuntu Desktop 24.04.5, directly verified |
| Linux ARM64 | `ahv-console_0.1.0_linux_arm64.tar.gz` | ARM64 Ubuntu Desktop VM, user reported |

Intel macOS is not qualified and is excluded from the supported release.
Details and evidence limits are in
[the qualification record](docs/verification/release-0.1.0.md).

Check your download, then extract it:

```sh
# Linux
sha256sum -c SHA256SUMS --ignore-missing
tar -xzf ahv-console_0.1.0_linux_amd64.tar.gz
./ahv-console_0.1.0_linux_amd64/ahv-console --version

# macOS: compare this output with the matching line in SHA256SUMS
shasum -a 256 ahv-console_0.1.0_darwin_arm64.tar.gz
tar -xzf ahv-console_0.1.0_darwin_arm64.tar.gz
./ahv-console_0.1.0_darwin_arm64/ahv-console --version
```

On Windows, use PowerShell:

```powershell
Get-FileHash .\ahv-console_0.1.0_windows_amd64.zip -Algorithm SHA256
Expand-Archive .\ahv-console_0.1.0_windows_amd64.zip -DestinationPath .
.\ahv-console_0.1.0_windows_amd64\ahv-console.exe --version
```

Compare the Windows hash with its checksum-manifest line. Copy the executable
to your PATH if desired. Only that executable is needed to run the application;
keep the accompanying licenses and source material when redistributing it.
The packages are not Developer ID signed/notarized or Authenticode signed.

Use a current browser supporting ES modules, Canvas, and WebSockets. Go 1.26
toolchain minimums are Windows 10/Server 2016, macOS 12, and Linux kernel 3.2;
these are toolchain baselines, not claims that every OS version was tested.

## Build from source

Release tooling uses Go 1.26.7 and Python 3.12+. Vendored browser assets and
notices are included in the source tree; Node.js is not required.

```sh
go mod download
go test ./...
go vet ./...
python3 tools/build.py 0.1.0
python3 tools/prepare-publication.py 0.1.0
python3 tools/verify-release.py publish/releases/v0.1.0
```

Raw binaries are in `dist/0.1.0/bin/`. The clean GitHub source export is in
`publish/github/`; platform and source release assets are in
`publish/releases/v0.1.0/`. See [publishing instructions](docs/publishing.md).
An optional `--experimental-macos-amd64` build is separate from supported
checksums/bundles and is not a claim of Intel Mac support.

## Usage

```sh
export AHV_CONSOLE_PC_HOST=pc.example.com
export AHV_CONSOLE_USERNAME=console-user
ahv-console 'Exact VM Name'
```

Missing host, username, and password are prompted only in a usable terminal.
Password entry is hidden. For unattended configuration, set
`AHV_CONSOLE_PASSWORD` privately in the process environment, then:

```sh
ahv-console 'Exact VM Name' --non-interactive --no-browser
ahv-console --vm-id 00000000-0000-4000-8000-000000000002 --non-interactive
ahv-console 'Exact VM Name' --cluster 'Exact Cluster Name'
```

PowerShell environment syntax is `$env:AHV_CONSOLE_PC_HOST = 'pc.example.com'`.
Avoid putting passwords in shell history or command arguments. No password flag
is supported. Credentials are held in process memory and are not persisted;
this does not promise guaranteed erasure of Go-managed memory.

| Flag | Environment variable | Default |
| --- | --- | --- |
| `--pc-host` | `AHV_CONSOLE_PC_HOST` | Prompt when interactive |
| `--pc-port` | `AHV_CONSOLE_PC_PORT` | `9440` |
| `--username` | `AHV_CONSOLE_USERNAME` | Prompt when interactive |
| None (password) | `AHV_CONSOLE_PASSWORD` | Hidden prompt when interactive |
| `--ca-cert` | `AHV_CONSOLE_CA_CERT` | System certificate trust |

Flags override environment values. Empty environment values count as missing;
invalid supplied values fail. Host is an IP address or hostname, without a URL
scheme or port. `--vm-id` is exclusive with a positional name and `--cluster`.
Name matching is exact and exhausts pagination. Duplicates list cluster and UUID
and require a valid interactive selection; unattended ambiguity fails.

Other flags: `--insecure`, `--non-interactive`, `--no-browser`, `--help`,
`--version`, `--licenses`. Informational commands work offline without PC
configuration. Diagnostics distinguish configuration, authentication, permission,
lookup, TLS, task, compatibility, and console failures and omit upstream secrets.

## TLS and permissions

TLS verification is enabled by default. To trust your enterprise CA:

```sh
ahv-console 'Exact VM Name' --ca-cert /path/to/enterprise-ca.pem
```

The PEM certificates augment system trust. `--insecure` explicitly disables
certificate verification **only for upstream PC connections** and prints a
warning. The local viewer uses HTTP bound to IPv4 loopback, with exact Host and
Origin validation, a single-use fragment bootstrap, and an HttpOnly,
SameSite=Strict authorization cookie. It proxies only the selected console.
This limits access by unrelated websites, not malicious same-user processes.

The PC user needs VM visibility, cluster visibility for name display/filtering,
task-read access, and **Generate VM Console Token** permission. See the official
references and observed endpoint/version table in
[PC 7.6 compatibility](docs/pc-7.6-compatibility.md). Initial authentication is
username/password; SSO/MFA and API keys are not implemented.

## Session lifetime

The command prints a local URL and attempts native browser launch unless
`--no-browser` is supplied. Launch failure leaves the URL available. Open the
URL once; subsequent refreshes use the local session cookie. Keep the process
in the foreground. Closing the tab releases its upstream socket; refreshing or
Reconnect works while PC accepts the existing authorization. If authorization
expires or is rejected, rerun the command. There is no automatic token renewal.
Ctrl+C cancels setup and closes the listener and active sockets. Concurrent
invocations have separate ports, selected VMs, and authorization secrets.
Process exit does not claim to revoke issued tokens on PC.

Runtime requires network access to PC, **not public Internet asset hosts**.
Dependency downloads and optional asset vendoring need public access only at
build time. Vendored assets are committed and embedded. Inspect their licenses
with `ahv-console --licenses`; source is available through the local viewer's
noVNC source link. See [asset provenance](docs/asset-provenance.md).

## License

Original code: [MIT](LICENSE), Copyright (c) 2026 Kurt Telep. Bundled dependencies
retain their own licenses, including noVNC's MPL-2.0. Every platform archive
contains full notices and the exact noVNC source. See
[third-party notices and redistribution details](THIRD_PARTY_NOTICES.md).
