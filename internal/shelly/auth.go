package shelly

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// DigestUser is the only user a Gen2+ device knows.
const DigestUser = "admin"

func basicAuth(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

// digestState is a server challenge plus our nonce counter.
type digestState struct {
	realm, nonce, qop, opaque, algorithm string
	nc                                   int
}

// parseChallenge reads `Digest qop="auth", realm="shellyplus1-…", nonce="…", algorithm=SHA-256`.
func parseChallenge(h string) (*digestState, bool) {
	scheme, rest, ok := strings.Cut(strings.TrimSpace(h), " ")
	if !ok || !strings.EqualFold(scheme, "Digest") {
		return nil, false
	}
	p := parseParams(rest)
	ds := &digestState{realm: p["realm"], nonce: p["nonce"], qop: p["qop"], opaque: p["opaque"], algorithm: p["algorithm"]}
	if ds.nonce == "" {
		return nil, false
	}
	if ds.algorithm == "" {
		ds.algorithm = "SHA-256"
	}
	if !strings.EqualFold(ds.algorithm, "SHA-256") {
		return nil, false // Shelly uses SHA-256 only
	}
	return ds, true
}

// parseParams splits `a="x, y", b=z` respecting quotes.
func parseParams(s string) map[string]string {
	out := map[string]string{}
	for len(s) > 0 {
		s = strings.TrimLeft(s, " ,")
		k, v, ok := strings.Cut(s, "=")
		if !ok {
			break
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimLeft(v, " ")
		if strings.HasPrefix(v, `"`) {
			end := strings.Index(v[1:], `"`)
			if end < 0 {
				out[k] = v[1:]
				break
			}
			out[k] = v[1 : end+1]
			s = v[end+2:]
		} else {
			val, tail, _ := strings.Cut(v, ",")
			out[k] = strings.TrimSpace(val)
			s = tail
		}
	}
	return out
}

func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// DigestResponse computes the RFC 7616 response with SHA-256:
// sha256(HA1:nonce:nc:cnonce:qop:HA2), HA1 = sha256(user:realm:password),
// HA2 = sha256(method:uri).
func DigestResponse(user, realm, password, method, uri, nonce, nc, cnonce, qop string) string {
	ha1 := sha256hex(user + ":" + realm + ":" + password)
	ha2 := sha256hex(method + ":" + uri)
	return sha256hex(ha1 + ":" + nonce + ":" + nc + ":" + cnonce + ":" + qop + ":" + ha2)
}

func (d *digestState) header(method, uri, password string) string {
	d.nc++
	nc := fmt.Sprintf("%08x", d.nc)
	var b [8]byte
	_, _ = rand.Read(b[:])
	cnonce := hex.EncodeToString(b[:])
	qop := "auth"
	resp := DigestResponse(DigestUser, d.realm, password, method, uri, d.nonce, nc, cnonce, qop)
	h := fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", algorithm=SHA-256, response="%s", qop=%s, nc=%s, cnonce="%s"`,
		DigestUser, d.realm, d.nonce, uri, resp, qop, nc, cnonce)
	if d.opaque != "" {
		h += fmt.Sprintf(`, opaque="%s"`, d.opaque)
	}
	return h
}
