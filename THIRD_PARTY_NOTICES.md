# Third-party notices and source availability

Original AHV Console code is licensed under the MIT license in `LICENSE`,
Copyright (c) 2026 Kurt Telep. That license does **not** replace the licenses of
the bundled third-party code listed below.

| Component | Pinned version/source | License / attribution |
| --- | --- | --- |
| noVNC core | 1.6.0, commit `a8dfd6a3ea3c74244f5ebdaa5a7f1023007a7820` | MPL-2.0; original file headers and AUTHORS retained |
| noVNC DES implementation | `core/crypto/des.js` at the same commit | Widget Workshop permission notice, AT&T attribution/disclaimer, Jef Poskanzer BSD-style notice; complete header reproduced |
| pako incorporated by noVNC | noVNC's ES-module adaptation based on pako 1.0.3 | MIT, Copyright (C) 2014–2016 Vitaly Puzrin |
| github.com/coder/websocket | v1.8.15 | ISC, Copyright (c) 2025 Coder |
| golang.org/x/term | v0.47.0 | BSD-3-Clause, The Go Authors |
| golang.org/x/sys | v0.49.0 | BSD-3-Clause, The Go Authors |
| Go runtime / standard library | Compiler identified in `release-manifest.json` | BSD-3-Clause, The Go Authors |

Full texts are in `internal/viewer/notices/` in source distributions and `licenses/`
in each platform release archive. `ahv-console --licenses` prints these texts
offline, including the project MIT license, pako's MIT notice, and the DES
attributions. Original third-party file headers are preserved.

## noVNC source delivery

The embedded noVNC modules are unmodified files from the pinned upstream source
archive. Its SHA-256 is:

`ebb8eabcd3670ce1a02e4025f03976826c289121cf631183647bd8325b003403`

The exact archive is available:

- In every platform bundle as `source/novnc-1.6.0-source.tar.gz`.
- As the separate release asset `novnc-1.6.0-source.tar.gz`.
- In the public source tree at `internal/viewer/assets/novnc/SOURCE.tar.gz`.
- From the running local viewer's **noVNC source** link.

The MPL-covered JavaScript files remain MPL-2.0; original Go code and the original
console page are separate MIT-covered files. If redistributing modified MPL
files, preserve their notices and make the corresponding modified Source Code
Form available under MPL-2.0. Distributing only a copied executable does not
remove its attribution or source-availability obligations: retain the notices
and provide recipients access to the matching source. Release bundles already
carry these materials without requiring a network download to obtain them.

The complete, unmodified upstream source archive additionally contains upstream
HTML/CSS under BSD-2-Clause, Orbitron font material under SIL OFL-1.1, and image
material under CC BY-SA 3.0 as identified by noVNC's `LICENSE.txt`. Those upstream
UI/fonts/images are not used by AHV Console's minimal runtime page. Their source
headers, notices, license texts/links, and attribution remain in the source
archive; supplemental upstream license texts are retained in `licenses/`.

See `docs/asset-provenance.md` for provenance and reproducible vendoring.
