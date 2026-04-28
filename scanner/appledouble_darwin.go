package scanner

import (
	"os"
	"path/filepath"
)

// PrepareMacOutputTree reduces AppleDouble sidecars on non-Apple filesystems.
func PrepareMacOutputTree() {
	_ = os.Setenv("COPYFILE_DISABLE", "1")
}

// CleanupAppleDoubleArtifacts removes macOS AppleDouble sidecars from the run tree.
func CleanupAppleDoubleArtifacts(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if ShouldSkipFileName(info.Name()) {
			_ = os.Remove(path)
		}
		return nil
	})
}
