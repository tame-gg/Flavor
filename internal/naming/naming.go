package naming

import (
	"crypto/sha256"
	"encoding/base32"
	"strings"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
)

const Suffix = "flavor.internal"

type Labels struct {
	Stable   string
	Friendly string
}

func (l Labels) Published() string {
	if l.Friendly != "" {
		return l.Friendly
	}
	return l.Stable
}

var hashEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

func ValidLabel(s string) bool {
	if s == "" || len(s) > 63 || s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}

func hashed(s string) string {
	sum := sha256.Sum256([]byte(s))
	return "ih-" + strings.ToLower(hashEncoding.EncodeToString(sum[:10]))
}

func StableNetworkLabel(id domain.NetworkID) string {
	l := strings.ToLower(string(id))
	if ValidLabel(l) && (l == string(id) || isULID(string(id))) {
		return l
	}
	return hashed(string(id))
}

func isULID(s string) bool {
	if len(s) != 26 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return true
}

func StableDeviceLabel(node domain.NodeID) string {
	s := string(node)
	if len(s) >= 1 && len(s) <= 60 {
		ok := true
		for _, r := range s {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
				ok = false
				break
			}
		}
		if ok {
			return "id-" + s
		}
	}
	return hashed(s)
}

func FriendlyNetworkCandidate(n domain.Network) string {
	return domain.NetworkLabel(n.DisplayName)
}

func FriendlyDeviceCandidate(d domain.Device) string {
	if h := strings.ToLower(d.Hostname); ValidLabel(h) {
		return h
	}
	if short, _, _ := strings.Cut(strings.ToLower(strings.TrimSuffix(d.DNSName, ".")), "."); ValidLabel(short) {
		return short
	}
	return domain.NetworkLabel(d.Hostname)
}

func reservedDeviceLabel(l string) bool {
	return strings.HasPrefix(l, "id-") || strings.HasPrefix(l, "ih-")
}

func NetworkLabels(nets []domain.Network) map[domain.NetworkID]Labels {
	out := make(map[domain.NetworkID]Labels, len(nets))
	stable := make(map[string]bool, len(nets))
	count := make(map[string]int, len(nets))
	for _, n := range nets {
		s := StableNetworkLabel(n.ID)
		stable[s] = true
		out[n.ID] = Labels{Stable: s}
		if f := FriendlyNetworkCandidate(n); f != "" {
			count[f]++
		}
	}
	for _, n := range nets {
		f := FriendlyNetworkCandidate(n)
		if f != "" && count[f] == 1 && !stable[f] && !reservedDeviceLabel(f) {
			l := out[n.ID]
			l.Friendly = f
			out[n.ID] = l
		}
	}
	return out
}

func DeviceLabels(devices []domain.Device) map[domain.NodeID]Labels {
	out := make(map[domain.NodeID]Labels, len(devices))
	count := make(map[string]int, len(devices))
	for _, d := range devices {
		out[d.ID.NodeID] = Labels{Stable: StableDeviceLabel(d.ID.NodeID)}
		if f := FriendlyDeviceCandidate(d); f != "" {
			count[f]++
		}
	}
	for _, d := range devices {
		f := FriendlyDeviceCandidate(d)
		if f != "" && count[f] == 1 && !reservedDeviceLabel(f) {
			l := out[d.ID.NodeID]
			l.Friendly = f
			out[d.ID.NodeID] = l
		}
	}
	return out
}

func Name(device, network string) string {
	if device == "" || network == "" {
		return ""
	}
	return device + "." + network + "." + Suffix
}

func Split(name string) (device, network string, ok bool) {
	rest, ok := strings.CutSuffix(name, "."+Suffix)
	if !ok {
		return "", "", false
	}
	device, network, ok = strings.Cut(rest, ".")
	if !ok || strings.Contains(network, ".") || !ValidLabel(device) || !ValidLabel(network) {
		return "", "", false
	}
	return device, network, true
}
