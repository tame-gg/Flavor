package inspect_test

import (
	"testing"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/inspect"
)

func prefer(dest string, kind domain.DestinationKind, net domain.NetworkID) *domain.DestinationPreference {
	return &domain.DestinationPreference{Destination: dest, Kind: kind, NetworkID: net}
}

func resolveWith(t *testing.T, raw string, pref *domain.DestinationPreference, nets ...inspect.Network) inspect.Result {
	t.Helper()
	q, err := inspect.ParseQuery(raw)
	if err != nil {
		t.Fatal(err)
	}
	return inspect.Resolve(q, nets, pref)
}

func TestPreferenceResolvesAmbiguousAddress(t *testing.T) {
	r := resolveWith(t, "100.64.0.1:22", prefer("100.64.0.1", domain.DestinationAddress, "A"), home, lunar)
	if r.Decision != inspect.DecisionUnique || r.Reason != inspect.ReasonDestinationPreference {
		t.Fatalf("%+v", r)
	}
	if r.Preference == nil || r.Preference.State != inspect.PreferenceApplied || r.Preference.Network.DisplayName != "LunarLabs" {
		t.Fatalf("%+v", r.Preference)
	}
	for _, c := range r.Candidates {
		want := inspect.StatusOutranked
		if c.Network.ID == "A" {
			want = inspect.StatusSelected
		}
		if c.Status != want {
			t.Fatalf("%s: %v", c.Network.DisplayName, c.Status)
		}
	}
}

func TestPreferenceOutranksAStrongerMatchElsewhere(t *testing.T) {
	byDNS := live("C", "Customer", dev("C", "1", "storage", "nas", "100.64.5.5"))
	byHost := live("B", "Home", dev("B", "1", "nas", "nas.home.ts.net", "100.64.0.9"))
	r := resolveWith(t, "nas", prefer("nas", domain.DestinationName, "B"), byDNS, byHost)
	if r.Decision != inspect.DecisionUnique || r.Reason != inspect.ReasonDestinationPreference {
		t.Fatalf("%+v", r)
	}
	for _, c := range r.Candidates {
		if (c.Network.ID == "B") != (c.Status == inspect.StatusSelected) {
			t.Fatalf("explicit preference must take precedence over match strength: %+v", c)
		}
	}
}

func TestPreferenceForDisconnectedNetworkIsNotApplied(t *testing.T) {
	off := inspect.Network{Network: domain.Network{ID: "A", DisplayName: "LunarLabs"}, State: domain.StateDisconnected}
	r := resolveWith(t, "100.64.0.1", prefer("100.64.0.1", domain.DestinationAddress, "A"), home, off)
	if r.Preference == nil || r.Preference.State != inspect.PreferenceNetworkNotConnected {
		t.Fatalf("%+v", r.Preference)
	}
	if r.Decision != inspect.DecisionUnique || r.Reason != inspect.ReasonExactDeviceAddress || r.Candidates[0].Network.ID != "B" {
		t.Fatalf("resolution without the preferred network falls back to normal matching: %+v", r)
	}
}

func TestPreferenceWithoutMatchOnPreferredNetwork(t *testing.T) {
	r := resolveWith(t, "100.64.0.9", prefer("100.64.0.9", domain.DestinationAddress, "A"), lunar, home)
	if r.Preference == nil || r.Preference.State != inspect.PreferenceNoMatchOnNetwork || r.Reason != inspect.ReasonExactDeviceAddress {
		t.Fatalf("%+v", r)
	}
}

func TestPreferredNetworkWithTwoMatchesStaysAmbiguousWithinIt(t *testing.T) {
	two := live("A", "LunarLabs", dev("A", "1", "a", "", "100.64.0.7"), dev("A", "2", "b", "", "100.64.0.7"))
	r := resolveWith(t, "100.64.0.7", prefer("100.64.0.7", domain.DestinationAddress, "A"), two, live("B", "Home", dev("B", "1", "c", "", "100.64.0.7")))
	if r.Decision != inspect.DecisionAmbiguous || r.Preference.State != inspect.PreferenceApplied {
		t.Fatalf("%+v", r)
	}
	for _, c := range r.Candidates {
		if (c.Network.ID == "A") != (c.Status == inspect.StatusTied) {
			t.Fatalf("%+v", c)
		}
	}
}

func TestPreferenceOfOtherKindOrDestinationIsIgnored(t *testing.T) {
	if r := resolveWith(t, "100.64.0.1", prefer("100.64.0.1", domain.DestinationName, "A"), home, lunar); r.Preference != nil || r.Decision != inspect.DecisionAmbiguous {
		t.Fatalf("%+v", r)
	}
	if r := resolveWith(t, "100.64.0.1", prefer("100.64.0.2", domain.DestinationAddress, "A"), home, lunar); r.Preference != nil {
		t.Fatalf("%+v", r)
	}
}

func TestConflictReportsPreferenceResolution(t *testing.T) {
	prefs := []domain.DestinationPreference{
		*prefer("100.64.0.1", domain.DestinationAddress, "A"),
		*prefer("postgres", domain.DestinationName, "Z"),
	}
	got := byID(inspect.Conflicts([]inspect.Network{lunar, home}, prefs))
	if got["address:100.64.0.1"].PreferredNetwork != "A" {
		t.Fatalf("%+v", got["address:100.64.0.1"])
	}
	if got["name:postgres"].PreferredNetwork != "" {
		t.Fatal("a preference for a network outside the conflict must not count as resolving it")
	}
}
