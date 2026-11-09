package builtin

import (
	"os"
	"testing"
)

func TestDescriptionSnapshots(t *testing.T) {
	texts := []string{descCurrentTime, descMemorySearch, descMemoryWrite, descMemoryForget, descToolResultRead}
	snap := ""
	for _, s := range texts {
		snap += s + "\n"
	}
	const file = "testdata/descriptions.snap"
	if os.Getenv("UPDATE_SNAPS") == "1" {
		_ = os.MkdirAll("testdata", 0755)
		if err := os.WriteFile(file, []byte(snap), 0644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(file)
	if err != nil {
		t.Skip("首次运行需 UPDATE_SNAPS=1")
	}
	if string(want) != snap {
		t.Fatal("Description 快照不匹配")
	}
}
