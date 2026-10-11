package service

import (
	"net/netip"
	"slices"
	"testing"
)

func prefixes(t *testing.T, raw ...string) []netip.Prefix {
	t.Helper()
	out := make([]netip.Prefix, 0, len(raw))
	for _, r := range raw {
		out = append(out, netip.MustParsePrefix(r))
	}
	return out
}

func TestParseRoutes(t *testing.T) {
	cases := []struct {
		name          string
		raw           []string
		rejectTailnet bool
		want          []string
		invalid       bool
	}{
		{name: "empty", want: []string{}},
		{name: "masks host bits", raw: []string{"192.168.1.77/24", "2001:db8::5/64"}, rejectTailnet: true, want: []string{"192.168.1.0/24", "2001:db8::/64"}},
		{name: "single host", raw: []string{"10.0.0.5/32"}, rejectTailnet: true, want: []string{"10.0.0.5/32"}},
		{name: "address without length", raw: []string{"10.0.0.5"}, invalid: true},
		{name: "garbage", raw: []string{"office"}, invalid: true},
		{name: "empty string", raw: []string{""}, invalid: true},
		{name: "default v4", raw: []string{"0.0.0.0/0"}, invalid: true},
		{name: "default v6", raw: []string{"::/0"}, invalid: true},
		{name: "default after masking", raw: []string{"10.0.0.0/0"}, invalid: true},
		{name: "tailnet v4", raw: []string{"100.64.0.0/10"}, rejectTailnet: true, invalid: true},
		{name: "inside tailnet v4", raw: []string{"100.100.1.0/24"}, rejectTailnet: true, invalid: true},
		{name: "covers tailnet v4", raw: []string{"100.0.0.0/8"}, rejectTailnet: true, invalid: true},
		{name: "tailnet v6", raw: []string{"fd7a:115c:a1e0:ab12::/64"}, rejectTailnet: true, invalid: true},
		{name: "next to tailnet v4", raw: []string{"100.128.0.0/16"}, rejectTailnet: true, want: []string{"100.128.0.0/16"}},
		{name: "tailnet allowed when removing", raw: []string{"100.64.0.0/10"}, want: []string{"100.64.0.0/10"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseRoutes(tc.raw, tc.rejectTailnet)
			if tc.invalid {
				if e, ok := err.(*Error); !ok || e.Code != CodeInvalidArgument {
					t.Fatalf("got %v, %v", got, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, prefixes(t, tc.want...)) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestMergeRoutes(t *testing.T) {
	cases := []struct {
		name                 string
		current, add, remove []string
		want                 []string
	}{
		{name: "add to nothing", add: []string{"10.1.0.0/16"}, want: []string{"10.1.0.0/16"}},
		{name: "add is idempotent", current: []string{"10.1.0.0/16"}, add: []string{"10.1.0.0/16", "10.1.0.0/16"}, want: []string{"10.1.0.0/16"}},
		{name: "remove absent is a no-op", current: []string{"10.1.0.0/16"}, remove: []string{"10.2.0.0/16"}, want: []string{"10.1.0.0/16"}},
		{name: "remove", current: []string{"10.1.0.0/16", "10.2.0.0/16"}, remove: []string{"10.1.0.0/16"}, want: []string{"10.2.0.0/16"}},
		{name: "remove wins over add", current: []string{"10.1.0.0/16"}, add: []string{"10.2.0.0/16"}, remove: []string{"10.2.0.0/16"}, want: []string{"10.1.0.0/16"}},
		{name: "sorted", add: []string{"192.168.0.0/24", "10.0.0.0/8", "2001:db8::/32", "10.0.0.0/16"}, want: []string{"10.0.0.0/8", "10.0.0.0/16", "192.168.0.0/24", "2001:db8::/32"}},
		{name: "remove everything", current: []string{"10.1.0.0/16"}, remove: []string{"10.1.0.0/16"}, want: []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			current := prefixes(t, tc.current...)
			before := slices.Clone(current)
			got := mergeRoutes(current, prefixes(t, tc.add...), prefixes(t, tc.remove...))
			if !slices.Equal(got, prefixes(t, tc.want...)) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			if !slices.Equal(current, before) {
				t.Fatalf("merge modified its input: %v", current)
			}
		})
	}
}
