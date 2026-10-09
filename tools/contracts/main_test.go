package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGeneratedInventoryAndBytes(t *testing.T) {
	root := t.TempDir()
	if err := run(false, root); err != nil {
		t.Fatal(err)
	}
	if err := run(true, root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "v1", "plugin.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run(true, root); err == nil {
		t.Fatal("drift must fail")
	}
	if err := run(false, root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "v1", "unexpected.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run(true, root); err == nil {
		t.Fatal("unowned artifact must fail")
	}
}
