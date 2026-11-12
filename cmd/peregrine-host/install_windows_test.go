//go:build windows

package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func deleteTree(t *testing.T, path string) {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.ENUMERATE_SUB_KEYS)
	if err == nil {
		subs, _ := k.ReadSubKeyNames(-1)
		k.Close()
		for _, s := range subs {
			deleteTree(t, path+`\`+s)
		}
	}
	if err := registry.DeleteKey(registry.CURRENT_USER, path); err != nil && !errors.Is(err, registry.ErrNotExist) {
		t.Logf("cleanup %s: %v", path, err)
	}
}

func readDefault(key string) (string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, key, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer k.Close()
	v, _, err := k.GetStringValue("")
	return v, err
}

func TestInstallUninstallRegistry(t *testing.T) {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	root := `Software\PeregrineTest\` + hex.EncodeToString(b)
	t.Cleanup(func() {
		deleteTree(t, root)
		// Remove the shared parent too if no other test run is using it.
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\PeregrineTest`)
	})
	cfg := t.TempDir()
	t.Setenv("PEREGRINE_HOME", cfg)
	t.Setenv(regRootEnv, root)

	if code := run([]string{"install", "--browsers", "all"}, nil, io.Discard, io.Discard); code != 0 {
		t.Fatalf("install exit %d", code)
	}
	manifest := filepath.Join(cfg, manifestFileName)
	if _, err := os.Stat(manifest); err != nil {
		t.Fatalf("central manifest: %v", err)
	}
	for _, tg := range targetsFor("windows", "", root, browsers) {
		v, err := readDefault(tg.RegKey)
		if err != nil || v != manifest {
			t.Fatalf("%s: %q %v", tg.RegKey, v, err)
		}
	}
	// The real browser keys must be untouched by a redirected install.
	if v, err := readDefault(`Software\Google\Chrome\NativeMessagingHosts\` + hostName); err == nil && v == manifest {
		t.Fatal("install escaped PEREGRINE_REG_ROOT")
	}

	if code := run([]string{"uninstall", "--browsers", "edge"}, nil, io.Discard, io.Discard); code != 0 {
		t.Fatalf("partial uninstall exit %d", code)
	}
	if _, err := readDefault(root + `\Microsoft\Edge\NativeMessagingHosts\` + hostName); err == nil {
		t.Fatal("edge key still present")
	}
	if _, err := os.Stat(manifest); err != nil {
		t.Fatal("partial uninstall must keep the manifest for the other browsers")
	}

	if code := run([]string{"uninstall"}, nil, io.Discard, io.Discard); code != 0 {
		t.Fatalf("uninstall exit %d", code)
	}
	for _, tg := range targetsFor("windows", "", root, browsers) {
		if _, err := readDefault(tg.RegKey); err == nil {
			t.Fatalf("%s still present", tg.RegKey)
		}
	}
	if _, err := os.Stat(manifest); !os.IsNotExist(err) {
		t.Fatal("manifest not removed")
	}
}
