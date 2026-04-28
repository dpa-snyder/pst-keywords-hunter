package scanner

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// flaggedPattern matches directory names containing privileged, confidential, or MOU.
var flaggedPattern = regexp.MustCompile(
	`(?i)(privil[ea]ged|confidential|\bmou\b|^mou[_\s\-]|[_\s\-]mou$)`,
)

// ScanFlaggedFolders walks the source tree and returns paths of directories
// whose names match privileged / confidential / MOU patterns.
func ScanFlaggedFolders(root string) ([]string, error) {
	var flagged []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable dirs
		}
		if info.IsDir() {
			if path != root && ShouldSkipWalkDir(path) {
				return filepath.SkipDir
			}
			name := filepath.Base(path)
			if IsFlaggedFolderName(name) {
				flagged = append(flagged, path)
			}
		}
		return nil
	})

	sort.Strings(flagged)
	return flagged, err
}

func IsFlaggedFolderName(name string) bool {
	return flaggedPattern.MatchString(name)
}
