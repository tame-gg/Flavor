package secret_test

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/secret"
)

func TestCanarySecretNeverAppearsInFormatsOrLogs(t *testing.T) {
	canary := "LATTICE_TEST_SECRET_DO_NOT_LEAK_7f3c9a"
	ctx := context.Background()
	store := secret.NewMemoryStore()
	ref, err := secret.NetworkRef(domain.NewNetworkID(), "provider-token")
	if err != nil {
		t.Fatal(err)
	}
	sec := secret.New(canary)
	if err := store.Set(ctx, ref, sec); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	logger := slog.New(slog.NewTextHandler(&b, nil))
	logger.Info("status",
		"secret", got,
		"err", fmt.Errorf("wrapped %s", sec.String()),
		"ref", ref.String(),
	)
	blobs := []string{
		b.String(),
		fmt.Sprintf("%v", sec),
		fmt.Sprintf("%#v", sec),
		sec.String(),
		sec.GoString(),
		ref.String(),
	}
	for _, blob := range blobs {
		if strings.Contains(blob, canary) {
			t.Fatalf("canary leaked in %q", blob)
		}
	}
	if got.Reveal() != canary {
		t.Fatal("reveal must still work")
	}
}
