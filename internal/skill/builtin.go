package skill

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed builtin
var builtinFS embed.FS

// InstallBuiltin copies embedded builtin skills to destDir/user/ if they don't already exist.
// Existing skills are never overwritten.
func InstallBuiltin(destDir string) (int, error) {
	userDir := filepath.Join(destDir, "user")
	count := 0

	err := fs.WalkDir(builtinFS, "builtin", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, "SKILL.md") {
			return nil
		}

		// path = "builtin/software-development/plan/SKILL.md"
		// rel  = "software-development/plan/SKILL.md"
		rel := strings.TrimPrefix(path, "builtin/")

		data, err := builtinFS.ReadFile(path)
		if err != nil {
			return err
		}

		target := filepath.Join(userDir, rel)
		if _, err := os.Stat(target); err == nil {
			return nil // already exists, don't overwrite
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0644); err != nil {
			return err
		}
		count++
		return nil
	})

	return count, err
}
