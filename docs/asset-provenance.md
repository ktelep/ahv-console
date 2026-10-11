# Embedded asset provenance

noVNC **1.6.0**, upstream commit
`a8dfd6a3ea3c74244f5ebdaa5a7f1023007a7820`, is embedded unmodified from:
`https://codeload.github.com/novnc/noVNC/tar.gz/a8dfd6a3ea3c74244f5ebdaa5a7f1023007a7820`.
Archive SHA-256:
`ebb8eabcd3670ce1a02e4025f03976826c289121cf631183647bd8325b003403`.

`tools/vendor-assets.py` verifies that checksum before extracting `core/` and
`vendor/` and copying notices. The upstream ES modules are served directly;
no transpiler or npm runtime is required. The source archive is embedded and
available from the viewer's source link to meet MPL source availability.
Retain source and applicable copyright/license notices with binary delivery.
The upstream license summary identifies MPL-2.0 and separately licensed code;
all provided license texts and original source file headers are retained.

The script also includes the original-code MIT license, licenses for pinned Go
dependencies and the Go runtime, pako's MIT license, and the complete DES
attribution header. `--licenses` prints these notices offline. Asset vendoring requires
public Internet access at build time; running a binary requires only PC network
access. Source archive extraction is limited to regular files and safe paths.

Each platform release bundle includes the exact source archive at
`source/novnc-1.6.0-source.tar.gz`, and the same archive is a separate release
asset. This makes matching Source Code Form available without requiring the
viewer to be running. `tools/verify-public.py` compares every bundled noVNC
`core/` and `vendor/` file with that archive and verifies its checksum.
Full attribution and license scope are described in `THIRD_PARTY_NOTICES.md`.
