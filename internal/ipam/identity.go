package ipam

import (
	"fmt"
	"net"
	"strings"
)

func normalizeMAC(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	mac, err := net.ParseMAC(raw)
	if err != nil || len(mac) != 6 {
		return "", fmt.Errorf("%w: MAC address must contain six octets", ErrInvalid)
	}
	return mac.String(), nil
}
