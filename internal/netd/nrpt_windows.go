package netd

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	nrptBase       = `SYSTEM\CurrentControlSet\Services\Dnscache\Parameters\DnsPolicyConfig`
	nrptRulePrefix = "{f1a7c0de-0000-4000-8000-"
	nrptPerRule    = 50
	nrptOverride   = 0x8
)

type NRPT struct{}

func nrptRule(i int) string { return fmt.Sprintf("%s%012d}", nrptRulePrefix, i) }

func (NRPT) Apply(_ context.Context, _ uint32, servers []netip.Addr, domains []string) error {
	var names []string
	for _, d := range domains {
		names = append(names, d, "."+d)
	}
	list := make([]string, len(servers))
	for i, a := range servers {
		list[i] = a.String()
	}
	rules := 0
	for ; len(names) > 0; rules++ {
		n := min(nrptPerRule, len(names))
		if err := writeRule(nrptRule(rules), strings.Join(list, "; "), names[:n]); err != nil {
			return err
		}
		names = names[n:]
	}
	if err := deleteRules(rules); err != nil {
		return err
	}
	return flushDNS()
}

func writeRule(id, servers string, names []string) error {
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, nrptBase+`\`+id, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return errors.Join(
		k.SetDWordValue("Version", 1),
		k.SetStringsValue("Name", names),
		k.SetStringValue("GenericDNSServers", servers),
		k.SetDWordValue("ConfigOptions", nrptOverride),
	)
}

func deleteRules(keep int) error {
	base, err := registry.OpenKey(registry.LOCAL_MACHINE, nrptBase, registry.ENUMERATE_SUB_KEYS)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	ids, err := base.ReadSubKeyNames(-1)
	base.Close()
	if err != nil {
		return err
	}
	var errs []error
	for _, id := range ids {
		var i int
		if !strings.HasPrefix(strings.ToLower(id), nrptRulePrefix) {
			continue
		}
		if _, err := fmt.Sscanf(id[len(nrptRulePrefix):], "%012d}", &i); err == nil && i < keep {
			continue
		}
		if err := registry.DeleteKey(registry.LOCAL_MACHINE, nrptBase+`\`+id); err != nil && !errors.Is(err, registry.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func flushDNS() error {
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	return exec.Command(filepath.Join(system, "ipconfig.exe"), "/flushdns").Run()
}

func (NRPT) Revert(context.Context, uint32) error {
	if err := deleteRules(0); err != nil {
		return err
	}
	return flushDNS()
}

func (NRPT) Generation(context.Context) (string, error) { return "", nil }
