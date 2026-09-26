//go:build linux || darwin || freebsd || netbsd || openbsd

package discovery

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// reuseControl lets us share UDP 5353 with the host's own mDNS responder
// (avahi, systemd-resolved) when running with host networking.
func reuseControl(network, address string, c syscall.RawConn) error {
	var serr error
	err := c.Control(func(fd uintptr) {
		if serr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); serr != nil {
			return
		}
		serr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
	})
	if err != nil {
		return err
	}
	return serr
}
