package bittorrentdht

import (
	"encoding/hex"
	"errors"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateUDPAddr(t *testing.T) {
	// Valid host and port
	addr, err := CreateUDPAddr("example.com", 6881)
	if assert.NoError(t, err) {
		assert.NotEmpty(t, addr)
		assert.NotEmpty(t, addr.String())
	}

	// Valid IPv4
	addr, err = CreateUDPAddr("127.0.0.1", 6881)
	if assert.NoError(t, err) {
		assert.NotEmpty(t, addr)
		assert.Equal(t, "127.0.0.1:6881", addr.String())
	}

	// Valid IPv6 (loopback address)
	addr, err = CreateUDPAddr("::1", 6881)
	if assert.NoError(t, err) {
		assert.NotEmpty(t, addr)
		assert.Equal(t, "[::1]:6881", addr.String())
	}

	// Valid IPv6
	addr, err = CreateUDPAddr("2606:4700:4700:0000:0000:0000:0000:1111", 6881)
	if assert.NoError(t, err) {
		assert.NotEmpty(t, addr)
		assert.Equal(t, "[2606:4700:4700::1111]:6881", addr.String())
	}

	// Valid IPv6 (compressed)
	addr, err = CreateUDPAddr("2606:4700:4700::1111", 6881)
	if assert.NoError(t, err) {
		assert.NotEmpty(t, addr)
		assert.Equal(t, "[2606:4700:4700::1111]:6881", addr.String())
	}

	// Invalid IPv4
	addr, err = CreateUDPAddr("1000.40.210.253", 6881)
	if assert.Error(t, err) {
		assert.Empty(t, addr)
	}

	// Invalid IPv6
	addr, err = CreateUDPAddr("2001:0db8:85a3:0000:0000:8a2e:0370:7334:3445", 6881)
	if assert.Error(t, err) {
		assert.Empty(t, addr)
	}

	// Invalid host
	addr, err = CreateUDPAddr("example", 1000)
	if assert.Error(t, err) {
		assert.Empty(t, addr)
	}

	// Empty host and port
	addr, err = CreateUDPAddr("", 0)
	if assert.Error(t, err) {
		assert.Empty(t, addr)
		assert.Equal(t, errors.New("Invalid host"), err)
	}

	// Invalid port
	addr, err = CreateUDPAddr("example.com", 0)
	if assert.Error(t, err) {
		assert.Empty(t, addr)
		assert.Equal(t, errors.New("Invalid port"), err)
	}

	// Invalid port
	addr, err = CreateUDPAddr("example.com", 75535)
	if assert.Error(t, err) {
		assert.Empty(t, addr)
		assert.Equal(t, errors.New("Invalid port"), err)
	}

	// Invalid port
	addr, err = CreateUDPAddr("example.com", -1000)
	if assert.Error(t, err) {
		assert.Empty(t, addr)
		assert.Equal(t, errors.New("Invalid port"), err)
	}
}

func TestCreateAddrPort(t *testing.T) {
	// Valid host and port
	addr, err := CreateAddrPort("example.com", 6881)
	if assert.NoError(t, err) {
		assert.NotEmpty(t, addr)
		assert.NotEmpty(t, addr.String())
	}

	// Valid IPv4
	addr, err = CreateAddrPort("127.0.0.1", 6881)
	if assert.NoError(t, err) {
		assert.NotEmpty(t, addr)
		assert.Equal(t, "127.0.0.1:6881", addr.String())
	}

	// Valid IPv6 (loopback address)
	addr, err = CreateAddrPort("::1", 6881)
	if assert.NoError(t, err) {
		assert.NotEmpty(t, addr)
		assert.Equal(t, "[::1]:6881", addr.String())
	}

	// Valid IPv6
	addr, err = CreateAddrPort("2606:4700:4700:0000:0000:0000:0000:1111", 6881)
	if assert.NoError(t, err) {
		assert.NotEmpty(t, addr)
		assert.Equal(t, "[2606:4700:4700::1111]:6881", addr.String())
	}

	// Valid IPv6 (compressed)
	addr, err = CreateAddrPort("2606:4700:4700::1111", 6881)
	if assert.NoError(t, err) {
		assert.NotEmpty(t, addr)
		assert.Equal(t, "[2606:4700:4700::1111]:6881", addr.String())
	}

	// Invalid IPv4
	addr, err = CreateAddrPort("1000.40.210.253", 6881)
	if assert.Error(t, err) {
		assert.Empty(t, addr)
	}

	// Invalid IPv6
	addr, err = CreateAddrPort("2001:0db8:85a3:0000:0000:8a2e:0370:7334:3445", 6881)
	if assert.Error(t, err) {
		assert.Empty(t, addr)
	}

	// Invalid host
	addr, err = CreateAddrPort("example", 1000)
	if assert.Error(t, err) {
		assert.Empty(t, addr)
	}

	// Empty host and port
	addr, err = CreateAddrPort("", 0)
	if assert.Error(t, err) {
		assert.Empty(t, addr)
		assert.Equal(t, errors.New("Invalid host"), err)
	}

	// Invalid port
	addr, err = CreateAddrPort("example.com", 0)
	if assert.Error(t, err) {
		assert.Empty(t, addr)
		assert.Equal(t, errors.New("Invalid port"), err)
	}

	// Invalid port
	addr, err = CreateAddrPort("example.com", 75535)
	if assert.Error(t, err) {
		assert.Empty(t, addr)
		assert.Equal(t, errors.New("Invalid port"), err)
	}

	// Invalid port
	addr, err = CreateAddrPort("example.com", -1000)
	if assert.Error(t, err) {
		assert.Empty(t, addr)
		assert.Equal(t, errors.New("Invalid port"), err)
	}
}

func TestCompareAddrPorts(t *testing.T) {
	addr1, _ := netip.ParseAddrPort("1.1.1.1:6881")
	addr2, _ := netip.ParseAddrPort("127.0.0.1:6881")
	addr3, _ := netip.ParseAddrPort("[::FFFF:127.0.0.1]:6881")

	assert.False(t, CompareAddrPorts(addr1, addr2))
	assert.False(t, CompareAddrPorts(addr1, addr3))

	assert.True(t, CompareAddrPorts(addr1, addr1))
	assert.True(t, CompareAddrPorts(addr2, addr3))
}

func TestDecodeId(t *testing.T) {
	id, _ := GenerateId()
	idStr := hex.EncodeToString(id)

	b, err := DecodeId(idStr)
	if assert.NoError(t, err) {
		assert.NotEmpty(t, b)
		assert.Len(t, b, 20)
		assert.Exactly(t, id, b)
	}

	b, err = DecodeId("abcd")
	if assert.NoError(t, err) {
		assert.NotEmpty(t, b)
		assert.Len(t, b, 2)
		assert.Exactly(t, []byte{171, 205}, b)
	}

	b, err = DecodeId("test")
	if assert.Error(t, err) {
		assert.Empty(t, b)
	}

	b, err = DecodeId("")
	if assert.Error(t, err) {
		assert.Empty(t, b)
		assert.Equal(t, errors.New("Invalid ID"), err)
	}
}

func TestGenerateId(t *testing.T) {
	id, err := GenerateId()
	if assert.NoError(t, err) {
		assert.NotEmpty(t, id)
		assert.Len(t, id, 20)
	}

	id1, _ := GenerateId()
	id2, _ := GenerateId()
	assert.NotEqual(t, id1, id2)
}

func TestGenerateRandomBytes(t *testing.T) {
	rb, err := GenerateRandomBytes(20)
	if assert.NoError(t, err) {
		assert.NotEmpty(t, rb)
		assert.Len(t, rb, 20)
	}

	rbs, err := GenerateRandomBytes(10)
	if assert.NoError(t, err) {
		assert.NotEmpty(t, rbs)
		assert.Len(t, rbs, 10)
	}

	r1, _ := GenerateRandomBytes(10)
	r2, _ := GenerateRandomBytes(10)
	assert.NotEqual(t, r1, r2)
}
