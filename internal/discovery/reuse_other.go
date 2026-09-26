//go:build !(linux || darwin || freebsd || netbsd || openbsd)

package discovery

import "syscall"

// reuseControl: address reuse is only needed where another responder may
// already hold UDP 5353; the container runs on Linux.
func reuseControl(network, address string, c syscall.RawConn) error { return nil }
