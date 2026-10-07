package synthetic

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"net/netip"
)

const (
	nsEmbeddedV4 uint16 = 0x0000
	nsAllocated  uint16 = 0x0001
	maxCounter          = 1<<48 - 1
)

var (
	ErrNotSynthetic   = errors.New("address is not a Lattice synthetic address")
	ErrInvalidIndex   = errors.New("network index must be between 1 and 65535")
	ErrCounterSpent   = errors.New("synthetic IPv6 counter exhausted")
	ErrInvalidPrefix  = errors.New("invalid Lattice ULA prefix")
	ErrNotIPv4        = errors.New("embedding requires an IPv4 target")
	ErrNotInNamespace = errors.New("address is in a reserved part of the network /64")
)

type Kind int

const (
	KindEmbeddedV4 Kind = iota + 1
	KindAllocatedV6
)

type Decoded struct {
	Index   uint16
	Kind    Kind
	V4      netip.Addr
	Counter uint64
}

func NewULA() (netip.Prefix, error) {
	var b [16]byte
	b[0] = 0xfd
	if _, err := rand.Read(b[1:6]); err != nil {
		return netip.Prefix{}, err
	}
	return netip.PrefixFrom(netip.AddrFrom16(b), 48), nil
}

func ValidULA(p netip.Prefix) bool {
	return p.IsValid() && p.Bits() == 48 && p.Addr().Is6() && p.Addr().As16()[0] == 0xfd && p.Masked() == p
}

func NetworkPrefix(ula netip.Prefix, index uint16) (netip.Prefix, error) {
	if !ValidULA(ula) {
		return netip.Prefix{}, ErrInvalidPrefix
	}
	if index == 0 {
		return netip.Prefix{}, ErrInvalidIndex
	}
	b := ula.Addr().As16()
	binary.BigEndian.PutUint16(b[6:8], index)
	return netip.PrefixFrom(netip.AddrFrom16(b), 64), nil
}

func ResolverAddress(ula netip.Prefix) netip.Addr {
	b := ula.Addr().As16()
	b[15] = 0x53
	return netip.AddrFrom16(b)
}

func EmbedV4(ula netip.Prefix, index uint16, v4 netip.Addr) (netip.Addr, error) {
	if !v4.Unmap().Is4() {
		return netip.Addr{}, ErrNotIPv4
	}
	p, err := NetworkPrefix(ula, index)
	if err != nil {
		return netip.Addr{}, err
	}
	b := p.Addr().As16()
	binary.BigEndian.PutUint16(b[8:10], nsEmbeddedV4)
	four := v4.Unmap().As4()
	copy(b[12:16], four[:])
	return netip.AddrFrom16(b), nil
}

func AllocatedV6(ula netip.Prefix, index uint16, counter uint64) (netip.Addr, error) {
	if counter == 0 || counter > maxCounter {
		return netip.Addr{}, ErrCounterSpent
	}
	p, err := NetworkPrefix(ula, index)
	if err != nil {
		return netip.Addr{}, err
	}
	b := p.Addr().As16()
	binary.BigEndian.PutUint16(b[8:10], nsAllocated)
	var c [8]byte
	binary.BigEndian.PutUint64(c[:], counter)
	copy(b[10:16], c[2:8])
	return netip.AddrFrom16(b), nil
}

func Decode(ula netip.Prefix, a netip.Addr) (Decoded, error) {
	if !ValidULA(ula) || !a.Is6() || !ula.Contains(a) {
		return Decoded{}, ErrNotSynthetic
	}
	b := a.As16()
	d := Decoded{Index: binary.BigEndian.Uint16(b[6:8])}
	if d.Index == 0 {
		return Decoded{}, ErrInvalidIndex
	}
	switch binary.BigEndian.Uint16(b[8:10]) {
	case nsEmbeddedV4:
		if b[10] != 0 || b[11] != 0 {
			return Decoded{}, ErrNotInNamespace
		}
		d.Kind, d.V4 = KindEmbeddedV4, netip.AddrFrom4([4]byte(b[12:16]))
	case nsAllocated:
		var c [8]byte
		copy(c[2:8], b[10:16])
		d.Kind, d.Counter = KindAllocatedV6, binary.BigEndian.Uint64(c[:])
		if d.Counter == 0 {
			return Decoded{}, ErrNotInNamespace
		}
	default:
		return Decoded{}, ErrNotInNamespace
	}
	return d, nil
}
