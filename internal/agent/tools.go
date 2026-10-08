package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/zendev-sh/goai"

	"github.com/mark/beevibe/internal/db/gen"
	"github.com/mark/beevibe/internal/files"
)

// Tool names (model-facing, exact).
const (
	toolListFiles  = "list_files"
	toolReadFile   = "read_file"
	toolWriteFile  = "write_file"
	toolDeleteFile = "delete_file"
)

// Tool descriptions (model-facing, exact).
const (
	descListFiles  = "List all files in the website directory with their sizes."
	descReadFile   = "Read a file from the website directory. Always read a file before changing it."
	descWriteFile  = "Create or overwrite a file with the complete new content. Send the whole file, never a fragment."
	descDeleteFile = "Delete a file from the website directory."
)

// pathInput is the shared input shape of the file tools.
type pathInput struct {
	Path string `json:"path" jsonschema:"description=Relative path of the file inside the website directory"`
}

// writeInput is the input shape of write_file.
type writeInput struct {
	Path    string `json:"path" jsonschema:"description=Relative path of the file to create or overwrite"`
	Content string `json:"content" jsonschema:"description=Complete new content of the file"`
}

// buildTools returns the four subdirectory-scoped tools (Q-AGENT-3). Every
// error they return comes from files.Store and therefore names only relative
// paths (T6). q is part of the frozen signature; the file tools need no SQL.
func buildTools(q *dbgen.Queries, fs *files.Store, roomID string, userID int64) []goai.Tool {
	return []goai.Tool{
		goai.NewTool(toolListFiles, descListFiles, func(_ context.Context, _ struct{}) (string, error) {
			return siteListing(fs, roomID, userID)
		}),
		goai.NewTool(toolReadFile, descReadFile, func(_ context.Context, in pathInput) (string, error) {
			data, err := fs.Read(roomID, userID, in.Path)
			if err != nil {
				return "", err
			}
			if int64(len(data)) > files.MaxFileBytes {
				return "", fmt.Errorf("file %q is larger than 256 KiB", in.Path)
			}
			return string(data), nil
		}),
		goai.NewTool(toolWriteFile, descWriteFile, func(_ context.Context, in writeInput) (string, error) {
			if err := fs.Write(roomID, userID, in.Path, []byte(in.Content)); err != nil {
				return "", err
			}
			return "wrote " + in.Path, nil
		}),
		goai.NewTool(toolDeleteFile, descDeleteFile, func(_ context.Context, in pathInput) (string, error) {
			if err := fs.Delete(roomID, userID, in.Path); err != nil {
				return "", err
			}
			return "deleted " + in.Path, nil
		}),
	}
}

// siteListing renders one "<path> (<size> bytes)" line per file, or "(empty)".
func siteListing(fs *files.Store, roomID string, userID int64) (string, error) {
	entries, err := fs.List(roomID, userID)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "(empty)", nil
	}
	var b strings.Builder
	for i, entry := range entries {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%s (%d bytes)", entry.Path, entry.Size)
	}
	return b.String(), nil
}
