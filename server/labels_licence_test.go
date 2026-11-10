package server

import (
	"os"
	"strings"
	"testing"
)

// The ISC licence of the iD presets asks for its notice in every copy of labels/presets.json.
func TestPresetsLicence(t *testing.T) {
	b, err := os.ReadFile("labels/LICENSE-id-tagging-schema.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"ISC License", "Copyright (c) iD Contributors", "this permission notice appear in all copies"} {
		if !strings.Contains(string(b), s) {
			t.Errorf("licence lacks %q", s)
		}
	}
}
