// Package files owns every byte stored under DATA_DIR/rooms and
// DATA_DIR/templates. It is the single place where the file policy (text-only
// whitelist, size caps, os.Root confinement, atomic writes) is enforced.
package files

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/mark/beevibe/internal/seed"
)

// File policy caps (Q-FILE-3).
const (
	MaxFiles     = 64
	MaxFileBytes = 256 * 1024
	MaxDirBytes  = 4 * 1024 * 1024
	maxPathLen   = 120
	tmpPrefix    = ".tmp-"
)

// allowedExt is the text-only whitelist (Q-FILE-1).
var allowedExt = map[string]bool{
	".html": true,
	".css":  true,
	".js":   true,
	".json": true,
	".svg":  true,
	".md":   true,
	".txt":  true,
}

// Entry describes one file in a user's subdirectory.
type Entry struct {
	Path string
	Size int64
}

// Store mediates access to the per-user site directories and the per-room
// templates.
type Store struct {
	dataDir      string
	maxFiles     int
	maxFileBytes int64
	maxDirBytes  int64
}

// New returns a Store rooted at dataDir.
func New(dataDir string) *Store {
	return &Store{
		dataDir:      dataDir,
		maxFiles:     MaxFiles,
		maxFileBytes: MaxFileBytes,
		maxDirBytes:  MaxDirBytes,
	}
}

// UserDir is the on-disk directory holding one user's site. It is built only
// from validated server-side values, never from model input.
func (s *Store) UserDir(roomID string, userID int64) string {
	return path.Join(s.dataDir, "rooms", roomID, strconv.FormatInt(userID, 10))
}

// TemplateDir is the on-disk directory holding a room's template.
func (s *Store) TemplateDir(roomID string) string {
	return path.Join(s.dataDir, "templates", roomID)
}

// OpenUser opens the user's directory as an os.Root, creating it if needed.
func (s *Store) OpenUser(roomID string, userID int64) (*os.Root, error) {
	dir := s.UserDir(roomID, userID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return os.OpenRoot(dir)
}

// OpenUserExisting opens an existing user directory without creating it. The
// public static route uses it so that requests for unknown users cannot grow
// the data directory.
func (s *Store) OpenUserExisting(roomID string, userID int64) (*os.Root, error) {
	return os.OpenRoot(s.UserDir(roomID, userID))
}

// RemoveUserDir deletes a user's whole subdirectory.
func (s *Store) RemoveUserDir(roomID string, userID int64) error {
	return os.RemoveAll(s.UserDir(roomID, userID))
}

// RemoveRoomDirs deletes every subdirectory of a room plus its template.
func (s *Store) RemoveRoomDirs(roomID string) error {
	if err := os.RemoveAll(path.Join(s.dataDir, "rooms", roomID)); err != nil {
		return err
	}
	return os.RemoveAll(s.TemplateDir(roomID))
}

// HasTemplate reports whether the room has a stored template.
func (s *Store) HasTemplate(roomID string) bool {
	info, err := os.Stat(s.TemplateDir(roomID))
	return err == nil && info.IsDir()
}

// List returns every file of a user's site, sorted by path.
func (s *Store) List(roomID string, userID int64) ([]Entry, error) {
	root, err := s.OpenUser(roomID, userID)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return listRoot(root)
}

func listRoot(root *os.Root) ([]Entry, error) {
	var entries []Entry
	err := fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasPrefix(d.Name(), tmpPrefix) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		entries = append(entries, Entry{Path: p, Size: info.Size()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

// Read returns the contents of one file.
func (s *Store) Read(roomID string, userID int64, name string) ([]byte, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	root, err := s.OpenUser(roomID, userID)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.ReadFile(name)
}

// Write stores data at name, atomically (temp file + rename) and within the
// policy caps.
func (s *Store) Write(roomID string, userID int64, name string, data []byte) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	if int64(len(data)) > s.maxFileBytes {
		return fmt.Errorf("file %q is %d bytes (limit %d)", name, len(data), s.maxFileBytes)
	}
	root, err := s.OpenUser(roomID, userID)
	if err != nil {
		return err
	}
	defer root.Close()

	entries, err := listRoot(root)
	if err != nil {
		return err
	}
	var total int64
	replacing := false
	count := 0
	for _, e := range entries {
		if e.Path == name {
			replacing = true
			continue // its old bytes do not count against the new total
		}
		total += e.Size
		count++
	}
	if !replacing {
		count++
	}
	if count > s.maxFiles {
		return fmt.Errorf("directory would hold %d files (limit %d)", count, s.maxFiles)
	}
	if total+int64(len(data)) > s.maxDirBytes {
		return fmt.Errorf("directory would hold %d bytes (limit %d)", total+int64(len(data)), s.maxDirBytes)
	}

	if dir := path.Dir(name); dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	tmp, err := tmpName()
	if err != nil {
		return err
	}
	f, err := root.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		root.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		root.Remove(tmp)
		return err
	}
	if err := root.Rename(tmp, name); err != nil {
		root.Remove(tmp)
		return err
	}
	return nil
}

// Delete removes one file.
func (s *Store) Delete(roomID string, userID int64, name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	root, err := s.OpenUser(roomID, userID)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.Remove(name)
}

// LOC counts every line (including blank) of every file except README.md
// (Q-UI-10).
func (s *Store) LOC(roomID string, userID int64) (int, error) {
	root, err := s.OpenUser(roomID, userID)
	if err != nil {
		return 0, err
	}
	defer root.Close()
	entries, err := listRoot(root)
	if err != nil {
		return 0, err
	}
	loc := 0
	for _, e := range entries {
		if e.Path == "README.md" {
			continue
		}
		data, err := root.ReadFile(e.Path)
		if err != nil {
			return 0, err
		}
		loc += countLines(data)
	}
	return loc, nil
}

func countLines(data []byte) int {
	n := bytes.Count(data, []byte("\n"))
	if len(data) > 0 && !bytes.HasSuffix(data, []byte("\n")) {
		n++
	}
	return n
}

// Seed wipes the user's subdirectory and repopulates it from the room template
// when one exists, otherwise from the built-in defaults. Missing index.html and
// README.md are backfilled from the defaults (Q-FILE-10).
func (s *Store) Seed(ctx context.Context, roomID string, userID int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := s.UserDir(roomID, userID)
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	files, err := s.templateContents(roomID)
	if err != nil {
		return err
	}
	if _, ok := files["index.html"]; !ok {
		files["index.html"] = seed.Default("index.html")
	}
	if _, ok := files["README.md"]; !ok {
		files["README.md"] = seed.Default("README.md")
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		if dir := path.Dir(name); dir != "." {
			if err := root.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		if err := root.WriteFile(name, files[name], 0o644); err != nil {
			return err
		}
	}
	return nil
}

// templateContents reads every file of the room template, or the embedded
// defaults when there is no template.
func (s *Store) templateContents(roomID string) (map[string][]byte, error) {
	if !s.HasTemplate(roomID) {
		return seed.Defaults()
	}
	root, err := os.OpenRoot(s.TemplateDir(roomID))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	out := map[string][]byte{}
	err = fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := root.ReadFile(p)
		if err != nil {
			return err
		}
		out[p] = data
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// PutTemplate validates the whole upload and then swaps it in atomically
// (AREDIT-10: a single bad file rejects the upload as a whole).
func (s *Store) PutTemplate(roomID string, files map[string][]byte) error {
	if err := ValidateSet(files); err != nil {
		return err
	}
	dst := s.TemplateDir(roomID)
	parent := path.Dir(dst)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	tmp := dst + ".tmp"
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return err
	}
	root, err := os.OpenRoot(tmp)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if dir := path.Dir(name); dir != "." {
			if err := root.MkdirAll(dir, 0o755); err != nil {
				root.Close()
				os.RemoveAll(tmp)
				return err
			}
		}
		if err := root.WriteFile(name, files[name], 0o644); err != nil {
			root.Close()
			os.RemoveAll(tmp)
			return err
		}
	}
	root.Close()
	if err := os.RemoveAll(dst); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	return nil
}

// DeleteTemplate removes the room template. Existing user subdirectories are
// left untouched.
func (s *Store) DeleteTemplate(roomID string) error {
	return os.RemoveAll(s.TemplateDir(roomID))
}

// ValidateName enforces the relative-path and extension rules for a single
// file. Every error message names only relative paths (T6).
func ValidateName(name string) error {
	if name == "" {
		return errors.New("file name must not be empty")
	}
	if len(name) > maxPathLen {
		return fmt.Errorf("file name is %d characters (limit %d)", len(name), maxPathLen)
	}
	if strings.ContainsAny(name, "\\\x00") {
		return fmt.Errorf("file %q: backslashes and NUL are not allowed", name)
	}
	if strings.HasPrefix(name, "/") {
		return fmt.Errorf("file %q: absolute paths are not allowed", name)
	}
	if strings.HasSuffix(name, "/") {
		return fmt.Errorf("file %q: trailing slash is not allowed", name)
	}
	if strings.HasPrefix(name, "./") {
		return fmt.Errorf("file %q: paths must not start with \"./\"", name)
	}
	if name != path.Clean(name) {
		return fmt.Errorf("file %q: path is not in canonical form", name)
	}
	for _, elem := range strings.Split(name, "/") {
		if elem == "." || elem == ".." {
			return fmt.Errorf("file %q: %q is not an allowed path element", name, elem)
		}
	}
	if !allowedExt[strings.ToLower(path.Ext(name))] {
		return fmt.Errorf("file %q is not an allowed type", name)
	}
	return nil
}

// ValidateSet enforces ValidateName on every file plus the set-wide caps. It is
// the gate for admin template uploads.
func ValidateSet(files map[string][]byte) error {
	if len(files) == 0 {
		return errors.New("no files were uploaded")
	}
	if len(files) > MaxFiles {
		return fmt.Errorf("upload would hold %d files (limit %d)", len(files), MaxFiles)
	}
	var total int64
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := ValidateName(name); err != nil {
			return err
		}
		size := int64(len(files[name]))
		if size > MaxFileBytes {
			return fmt.Errorf("file %q is %d bytes (limit %d)", name, size, MaxFileBytes)
		}
		total += size
	}
	if total > MaxDirBytes {
		return fmt.Errorf("upload would hold %d bytes (limit %d)", total, MaxDirBytes)
	}
	return nil
}

func tmpName() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return tmpPrefix + hex.EncodeToString(b[:]), nil
}
