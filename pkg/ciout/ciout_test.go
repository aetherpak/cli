package ciout

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEmitToFileAppends(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "out.env")
	if err := Emit(p, []KV{{"app-id", "org.example.App"}, {"arch", "x86_64"}}); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if err := Emit(p, []KV{{"branch", "stable"}}); err != nil {
		t.Fatalf("emit append: %v", err)
	}
	got, _ := os.ReadFile(p)
	want := "app-id=org.example.App\narch=x86_64\nbranch=stable\n"
	if string(got) != want {
		t.Fatalf("got %q want %q", string(got), want)
	}
}

func TestEmitRejectsNewlineValue(t *testing.T) {
	if err := Emit(filepath.Join(t.TempDir(), "o"), []KV{{"k", "a\nb"}}); err == nil {
		t.Fatal("expected error for multiline value")
	}
}

func TestEmitStdoutWhenEmptyOrDash(t *testing.T) {
	// Empty path and "-" must not error and must not create a file.
	if err := Emit("", []KV{{"k", "v"}}); err != nil {
		t.Fatalf("empty path: %v", err)
	}
	if err := Emit("-", []KV{{"k", "v"}}); err != nil {
		t.Fatalf("dash path: %v", err)
	}
}
