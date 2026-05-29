// Package ciout writes CI-agnostic dotenv KEY=VALUE output. Both GitHub
// ($GITHUB_OUTPUT) and GitLab (artifacts:reports:dotenv) consume this directly.
package ciout

import (
	"fmt"
	"os"
	"strings"
)

type KV struct {
	Key   string
	Value string
}

// Emit writes pairs as `key=value` lines. An empty path or "-" writes to stdout;
// any other path is created/appended. Values must be single-line.
func Emit(path string, pairs []KV) error {
	var b strings.Builder
	for _, p := range pairs {
		if strings.ContainsAny(p.Value, "\n\r") {
			return fmt.Errorf("ciout: value for %q contains a newline", p.Key)
		}
		fmt.Fprintf(&b, "%s=%s\n", p.Key, p.Value)
	}
	if path == "" || path == "-" {
		_, err := os.Stdout.WriteString(b.String())
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(b.String())
	return err
}
