// Package shellylanman holds the repository documents the About page shows,
// embedded in the binary: release notes, licence and third-party notices.
package shellylanman

import _ "embed"

// Changelog is CHANGELOG.md.
//
//go:embed CHANGELOG.md
var Changelog string

// License is the GPL-3.0 text (LICENSE).
//
//go:embed LICENSE
var License string

// Notices is THIRD_PARTY_NOTICES.md.
//
//go:embed THIRD_PARTY_NOTICES.md
var Notices string
