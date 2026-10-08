// Package seed holds the built-in default website that every user
// subdirectory is populated with when their room has no template.
package seed

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed README.md index.html
var FS embed.FS

// Defaults returns the built-in default files keyed by base name
// ("README.md" and "index.html").
func Defaults() (map[string][]byte, error) {
	entries, err := fs.ReadDir(FS, ".")
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := FS.ReadFile(e.Name())
		if err != nil {
			return nil, err
		}
		out[e.Name()] = data
	}
	if _, ok := out["index.html"]; !ok {
		return nil, fmt.Errorf("seed: embedded index.html missing")
	}
	if _, ok := out["README.md"]; !ok {
		return nil, fmt.Errorf("seed: embedded README.md missing")
	}
	return out, nil
}

// Default returns the single built-in default file, or nil when absent.
func Default(name string) []byte {
	data, err := FS.ReadFile(name)
	if err != nil {
		return nil
	}
	return data
}
