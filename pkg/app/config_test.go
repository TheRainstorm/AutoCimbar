package app

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadINIConfigMergesDefaultAndCommandSection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	path := filepath.Join(home, DefaultConfigPath)
	if err := os.WriteFile(path, []byte(`
Q = 80
cell = 8t4s2c

[encoder]
Q = 120
fps = 200

[decoder]
Q = 60
`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	values, err := LoadINIConfig("encoder")
	if err != nil {
		t.Fatalf("LoadINIConfig: %v", err)
	}
	if values["Q"] != "120" || values["cell"] != "8t4s2c" || values["fps"] != "200" {
		t.Fatalf("encoder values = %#v", values)
	}

	values, err = LoadINIConfig("decoder")
	if err != nil {
		t.Fatalf("LoadINIConfig decoder: %v", err)
	}
	if values["Q"] != "60" || values["cell"] != "8t4s2c" {
		t.Fatalf("decoder values = %#v", values)
	}
}

func TestApplyINIConfigLetsCommandLineOverrideConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	path := filepath.Join(home, DefaultConfigPath)
	if err := os.WriteFile(path, []byte(`
Q = 80
cell = 4t4s8c
packets = 8
`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	q := fs.Int("Q", 120, "")
	cell := fs.String("cell", "", "")
	packets := fs.Int("packets", 1, "")
	pShort := fs.Int("p", 0, "")
	if err := fs.Parse([]string{"-Q", "100", "-p", "3"}); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	aliases := map[string][]string{"packets": {"p"}}
	if err := ApplyINIConfig(fs, "encoder", aliases); err != nil {
		t.Fatalf("ApplyINIConfig: %v", err)
	}
	if *q != 100 {
		t.Fatalf("Q = %d, want command-line override 100", *q)
	}
	if *cell != "4t4s8c" {
		t.Fatalf("cell = %q, want config value", *cell)
	}
	if *packets != 1 || *pShort != 3 {
		t.Fatalf("packets=%d p=%d, want short command-line override before alias merge", *packets, *pShort)
	}
}
