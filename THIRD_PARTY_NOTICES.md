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
| `golang.org/x/net` | v0.58.0 | BSD-3-Clause (mDNS message parsing, IPv4 multicast) |
| `golang.org/x/sys` | v0.47.0 | BSD-3-Clause (socket options for sharing UDP 5353) |
| `github.com/skip2/go-qrcode` | v0.0.0-20200617195104-da1b6568686e | MIT (QR code of the local firmware download) |

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


`github.com/skip2/go-qrcode` licence:

```
Copyright (c) 2014 Tom Harwood

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
```

`golang.org/x/net` and `golang.org/x/sys` licence (identical text):

```
Copyright 2009 The Go Authors.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.
   * Neither the name of Google LLC nor the names of its
contributors may be used to endorse or promote products derived from
this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

## JavaScript bundled into the frontend

The script editor (Scripts dialog) is built on CodeMirror 6; it is bundled into
a separate file that is loaded only when the editor opens.

| Package | Version | Licence |
|---|---|---|
| `codemirror` | 6.0.2 | MIT |
| `@codemirror/autocomplete` | 6.20.3 | MIT |
| `@codemirror/commands` | 6.11.1 | MIT |
| `@codemirror/lang-javascript` | 6.2.5 | MIT |
| `@codemirror/language` | 6.12.4 | MIT |
| `@codemirror/lint` | 6.9.7 | MIT |
| `@codemirror/search` | 6.7.2 | MIT |
| `@codemirror/state` | 6.7.6 | MIT |
| `@codemirror/theme-one-dark` | 6.1.3 | MIT |
| `@codemirror/view` | 6.43.13 | MIT |
| `@lezer/common`, `@lezer/highlight`, `@lezer/javascript`, `@lezer/lr` | 1.5.3, 1.2.4, 1.5.5, 1.4.10 | MIT |
| `@marijn/find-cluster-break`, `crelt`, `style-mod`, `w3c-keyname` | 1.0.4, 1.0.7, 4.1.4, 2.2.8 | MIT |

All by Marijn Haverbeke and others, under this licence (copyright years
2016–2024 per package):

```
MIT License

Copyright (C) 2018-2021 by Marijn Haverbeke <marijn@haverbeke.berlin> and others

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
```

---

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
