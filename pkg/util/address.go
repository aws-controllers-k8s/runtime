package util

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
)

// GetHostPort extracts the host and port from a given address string.
func GetHostPort(address string) (string, int, error) {
	u, err := url.Parse(fmt.Sprintf("//%s", address))
	if err != nil {
		return "", 0, err
	}

	host, portString, err := net.SplitHostPort(u.Host)
	if err != nil {
		return "", 0, err
	}

	port, err := strconv.Atoi(portString)
	if err != nil {
		return "", 0, fmt.Errorf("cannot parse port: %v", err)
	}

	return host, port, nil
}
