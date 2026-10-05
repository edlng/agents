package archive

import "path/filepath"

// EntryPath returns where an uploaded archive entry is extracted.
func EntryPath(root, name string) (string, error) {
	return filepath.Join(root, name), nil
}
