package scanner

import (
	"fmt"
	"os/exec"
	"strings"
)

// HasReadPST checks if readpst is available on the system PATH.
func HasReadPST() bool {
	_, ok := dependencyPath("readpst")
	return ok
}

// ExtractPST runs readpst to extract a PST/OST file into EML files.
// Returns nil on success. The extracted EMLs go into tmpDir.
func ExtractPST(pstPath string, tmpDir string, logger *Logger) error {
	readpst, ok := dependencyPath("readpst")
	if !ok {
		return fmt.Errorf("readpst not found on PATH")
	}

	// -S: save individual emails  -D: include deleted items
	// -M: write emails in MIME format  -e: output as .eml
	// -o: output directory
	cmd := exec.Command(readpst, "-S", "-D", "-M", "-e", "-o", tmpDir, pstPath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		outStr := strings.TrimSpace(string(output))
		if outStr != "" && logger != nil {
			logger.Log(fmt.Sprintf("  readpst output: %s", outStr))
		}
		return fmt.Errorf("readpst failed: %w", err)
	}
	return nil
}
