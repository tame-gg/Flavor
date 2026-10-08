package synthetic

import (
	"bufio"
	"encoding/hex"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
)

func LocalPrefixes() []netip.Prefix {
	var out []netip.Prefix
	if f, err := os.Open("/proc/net/route"); err == nil {
		out = append(out, parseRoute4(f)...)
		f.Close()
	}
	if f, err := os.Open("/proc/net/ipv6_route"); err == nil {
		out = append(out, parseRoute6(f)...)
		f.Close()
	}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if p, err := netip.ParsePrefix(a.String()); err == nil {
				out = append(out, p.Masked())
			}
		}
	}
	return out
}

func parseRoute4(r interface{ Read([]byte) (int, error) }) []netip.Prefix {
	var out []netip.Prefix
	sc := bufio.NewScanner(r)
	sc.Scan()
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 8 {
			continue
		}
		dst, err1 := leAddr(f[1])
		mask, err2 := leAddr(f[7])
		if err1 != nil || err2 != nil {
			continue
		}
		bits, _ := net.IPMask(mask.AsSlice()).Size()
		out = append(out, netip.PrefixFrom(dst, bits).Masked())
	}
	return out
}

func leAddr(h string) (netip.Addr, error) {
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return netip.Addr{}, err
	}
	return netip.AddrFrom4([4]byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)}), nil
}

func parseRoute6(r interface{ Read([]byte) (int, error) }) []netip.Prefix {
	var out []netip.Prefix
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 2 {
			continue
		}
		raw, err := hex.DecodeString(f[0])
		bits, err2 := strconv.ParseUint(f[1], 16, 8)
		if err != nil || err2 != nil || len(raw) != 16 {
			continue
		}
		out = append(out, netip.PrefixFrom(netip.AddrFrom16([16]byte(raw)), int(bits)).Masked())
	}
	return out
}
