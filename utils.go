package bittorrentdht

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"net"
	"net/netip"
	"strconv"
)

// CreateUDPAddr creates a *net.UDPAddr from a host and port.
// It will return an error if the host is not a valid IP address, or the port is
// not a valid port number.
func CreateUDPAddr(host string, port int) (*net.UDPAddr, error) {
	if len(host) == 0 {
		return nil, errors.New("Invalid host")
	}

	if port <= 0 || port > 65535 {
		return nil, errors.New("Invalid port")
	}

	return net.ResolveUDPAddr("udp", net.JoinHostPort(host, strconv.Itoa(port)))
}

// CreateAddrPort creates a netip.AddrPort from a host and port.
// It will return an error if the host is not a valid IP address, or the port is
// not a valid port number.
func CreateAddrPort(host string, port int) (netip.AddrPort, error) {
	addrPort := netip.AddrPort{}

	if len(host) == 0 {
		return addrPort, errors.New("Invalid host")
	}

	if port <= 0 || port > 65535 {
		return addrPort, errors.New("Invalid port")
	}

	if net.ParseIP(host) == nil {
		ip, err := net.LookupIP(host)
		if err != nil {
			return addrPort, err
		}

		host = ip[0].String()
	}

	return netip.ParseAddrPort(net.JoinHostPort(host, strconv.Itoa(port)))
}

// CompareAddrPorts compares two netip.AddrPorts.
// It will return true if the two AddrPorts are equal, false otherwise.
func CompareAddrPorts(a, b netip.AddrPort) bool {
	if a.Addr().Unmap().Compare(b.Addr().Unmap()) == 0 && a.Port() == b.Port() {
		return true
	}

	return false
}

// DecodeId decodes a hex string (SHA-1) into a byte slice.
// It will return an error if the string is not a valid hex string.
func DecodeId(id string) ([]byte, error) {
	if len(id) == 0 {
		return nil, errors.New("Invalid ID")
	}

	dec, err := hex.DecodeString(id)
	if err != nil {
		return nil, err
	}

	return dec, nil
}

// GenerateId generates a random 160-bit ID (SHA-1).
// It will return an error if the system's secure random number generator fails
// to function correctly, in which case the caller should not continue.
func GenerateId() ([]byte, error) {
	b, err := GenerateRandomBytes(20)
	if err != nil {
		return nil, err
	}

	h := sha1.New()
	h.Write(b)

	return h.Sum(nil), nil
}

// GenerateRandomBytes returns securely generated random bytes.
// It will return an error if the system's secure random number generator fails
// to function correctly, in which case the caller should not continue.
func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
	}

	return b, nil
}
