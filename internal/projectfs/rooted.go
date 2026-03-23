package projectfs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func ExistsInRoot(root, rel string) bool {
	_, err := ReadFileInRoot(root, rel)
	return err == nil
}

func ReadFileInRoot(root, rel string) ([]byte, error) {
	base := filepath.Clean(root)
	path := filepath.Clean(filepath.Join(base, rel))

	relPath, err := filepath.Rel(base, path)
	if err != nil {
		return nil, err
	}
	if relPath == ".." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("path escapes project root: %s", rel)
	}

	// #nosec G304 -- path is normalized and constrained to project root above.
	return os.ReadFile(path)
}
