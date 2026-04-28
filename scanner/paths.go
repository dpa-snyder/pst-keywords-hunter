package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var runDirPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}_\d{6}$`)

func RelPath(root, target string) string {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return filepath.ToSlash(target)
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return filepath.ToSlash(target)
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return filepath.ToSlash(targetAbs)
	}
	if rel == "." {
		return "."
	}
	return filepath.ToSlash(rel)
}

func ValidateRoots(sourceDir, outputDir string) error {
	sourceAbs, err := filepath.Abs(sourceDir)
	if err != nil {
		return fmt.Errorf("failed to resolve source directory: %w", err)
	}
	outputAbs, err := filepath.Abs(outputDir)
	if err != nil {
		return fmt.Errorf("failed to resolve output directory: %w", err)
	}

	if sourceAbs == outputAbs {
		return fmt.Errorf("output directory must be outside the source directory")
	}

	sourcePrefix := sourceAbs + string(os.PathSeparator)
	outputPrefix := outputAbs + string(os.PathSeparator)
	if strings.HasPrefix(outputAbs, sourcePrefix) || strings.HasPrefix(sourceAbs, outputPrefix) {
		return fmt.Errorf("source and output directories must be separate root folders")
	}

	return nil
}

func ShouldSkipWalkDir(path string) bool {
	base := filepath.Base(path)
	switch strings.ToLower(base) {
	case "unknown_date", "$recycle.bin", ".trashes", ".spotlight-v100", ".fseventsd":
		return true
	}
	if runDirPattern.MatchString(base) {
		return true
	}
	return false
}

func ShouldSkipFileName(name string) bool {
	if strings.HasPrefix(name, "~$") {
		return true
	}
	switch strings.ToLower(name) {
	case ".ds_store", "thumbs.db", "desktop.ini":
		return true
	}
	return strings.HasPrefix(name, "._")
}

func SafeJoinRelative(rel string) string {
	if rel == "." || rel == "" {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i, part := range parts {
		if part == "" || part == "." {
			parts[i] = ""
			continue
		}
		parts[i] = SanitizePathSegment(part)
	}
	var cleaned []string
	for _, part := range parts {
		if part != "" {
			cleaned = append(cleaned, part)
		}
	}
	return filepath.Join(cleaned...)
}
