# Third-party notices

ShellyLanMan is licensed under GPL-3.0-or-later (see `LICENSE`). It builds on
the work below. **Rule: a licence is recorded here before the asset or
dependency it covers is committed.**

---

## ShellyScanner — GPL-3.0

ShellyLanMan is based on ShellyScanner by Antonio Flaccomio (usnasoft),
https://github.com/usnasoft/shellyscanner, distributed under the GNU General
Public License v3.0 (its `COPYING` file). ShellyScanner is the functional and
code reference; code that is ported from it is marked in its file header and
listed in `docs/PROVENANCE.md`. ShellyLanMan is an independent project, not
ShellyScanner and not endorsed by its author.

---

## MikroDash — MIT

Design tokens and named colour palettes (`web/public/app.css`, marked block)
and the appearance logic (`web/src/appearance.ts`, adapted) come from
MikroDash, https://github.com/SecOps-7/MikroDash.

```
MIT License

Copyright (c) 2026 MikroDash

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

---

## Fonts — SIL Open Font License 1.1

`web/public/fonts/`: Inter, Oxanium, IBM Plex Sans, Nunito, Roboto, JetBrains
Mono (weights 400–700, woff2), taken from MikroDash's self-hosted set. The
per-family copyright notices and the full licence text are in
`web/public/fonts/OFL.txt`, which is shipped with the files and served at
`/fonts/OFL.txt`.

---

## Go modules linked into the binary

| Module | Version | Licence |
|---|---|---|
| Go standard library | 1.27 | BSD-3-Clause |
| `github.com/coder/websocket` | v1.8.15 | ISC |

`github.com/coder/websocket` licence:

```
Copyright (c) 2025 Coder

Permission to use, copy, modify, and distribute this software for any
purpose with or without fee is hereby granted, provided that the above
copyright notice and this permission notice appear in all copies.

THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
```

## Build-time only (not shipped in the image)

| Tool | Licence |
|---|---|
| TypeScript | Apache-2.0 |
| esbuild | MIT |
| @types/node | MIT |

---

## Trademark

Shelly is a trademark of its owner (Shelly Group). ShellyLanMan is not
affiliated with or endorsed by Shelly Group.
