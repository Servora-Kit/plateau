package security

import (
	"net/netip"
	"strings"
)

// IsLoopbackHost reports whether host is an IP loopback literal (127.0.0.0/8 or ::1)
// or a RFC 6761 localhost name ("localhost" or non-empty "*.localhost").
func IsLoopbackHost(host string) bool {
	h := strings.TrimSpace(host)
	if h == "" {
		return false
	}
	if addr, err := netip.ParseAddr(h); err == nil {
		return addr.Unmap().IsLoopback()
	}
	h = strings.ToLower(h)
	return h == "localhost" || (strings.HasSuffix(h, ".localhost") && !strings.HasPrefix(h, ".") && !strings.Contains(h, ".."))
}
