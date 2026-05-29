package repoinfo

import "testing"

func TestParseRef(t *testing.T) {
	id, arch, branch, err := parseRef("app/org.example.App/x86_64/stable")
	if err != nil || id != "org.example.App" || arch != "x86_64" || branch != "stable" {
		t.Fatalf("got %q %q %q err=%v", id, arch, branch, err)
	}
	if _, _, _, err := parseRef("not/an/app/ref"); err == nil {
		t.Fatal("expected error for non-app ref")
	}
	if _, _, _, err := parseRef("app/too/few"); err == nil {
		t.Fatal("expected error for malformed ref")
	}
}
