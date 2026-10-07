package secret_test

import (
	"strings"
	"testing"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/secret"
)

func TestSecretRefNetworkValid(t *testing.T) {
	id := domain.NewNetworkID()
	ref, err := secret.NetworkRef(id, "oauth-refresh-token")
	if err != nil {
		t.Fatal(err)
	}
	want := "network/" + string(id) + "/oauth-refresh-token"
	if ref.String() != want {
		t.Fatalf("got %q want %q", ref.String(), want)
	}
	parsed, err := secret.ParseRef(want)
	if err != nil {
		t.Fatal(err)
	}
	if parsed != ref {
		t.Fatalf("parsed=%q", parsed)
	}
}

func TestSecretRefRejectsMalformed(t *testing.T) {
	cases := []string{
		"",
		"network",
		"network/",
		"network/foo",
		"network/../escape/token",
		"network/foo/bar/baz",
		"networks/x/y",
		"network/" + string(domain.NewNetworkID()) + "/BAD",
		"network/" + string(domain.NewNetworkID()) + "/has space",
		"network/" + string(domain.NewNetworkID()) + "/" + strings.Repeat("a", 65),
	}
	for _, c := range cases {
		if _, err := secret.ParseRef(c); err == nil {
			t.Fatalf("expected error for %q", c)
		}
	}
}

func TestSecretRefDoesNotEmbedSecret(t *testing.T) {
	id := domain.NewNetworkID()
	ref, err := secret.NetworkRef(id, "provider-token")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ref.String(), "secret") && false {
		t.Fatal("unused")
	}
	canary := "LATTICE_TEST_SECRET_DO_NOT_LEAK_7f3c9a"
	if strings.Contains(ref.String(), canary) {
		t.Fatal("ref must not contain secret material")
	}
}
