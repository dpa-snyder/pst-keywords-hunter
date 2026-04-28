package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Logger writes to both a file and collects lines for the TUI.
type Logger struct {
	file *os.File
	path string
}

// NewLogger creates a logger writing to script_log.txt in the output directory.
func NewLogger(outputDir string) (*Logger, error) {
	path := filepath.Join(outputDir, "script_log.txt")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	return &Logger{file: f, path: path}, nil
}

// Log writes a line to the log file.
func (l *Logger) Log(msg string) {
	if l.file != nil {
		fmt.Fprintln(l.file, msg)
		l.file.Sync()
	}
}

// Close the log file.
func (l *Logger) Close() {
	if l.file != nil {
		l.file.Close()
	}
}

// WriteFlaggedFolders writes flagged_folders.txt in the output directory.
func WriteFlaggedFolders(outputDir string, flagged []string) error {
	path := filepath.Join(outputDir, "flagged_folders.txt")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintln(f, "# Folders flagged for post-processing review")
	fmt.Fprintln(f, "# Matched patterns: privileged, confidential, MOU")
	fmt.Fprintln(f)
	for _, folder := range flagged {
		fmt.Fprintln(f, folder)
	}
	return nil
}

var unsafeChars = regexp.MustCompile(`[^\w\-]`)
var multiSeparator = regexp.MustCompile(`-+`)

// SanitizeName converts a source filename to a safe directory component.
func SanitizeName(filename string) string {
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	return SanitizePathSegment(stem)
}

func SanitizePathSegment(value string) string {
	safe := strings.ToLower(strings.TrimSpace(value))
	safe = strings.ReplaceAll(safe, " ", "-")
	safe = unsafeChars.ReplaceAllString(safe, "-")
	safe = multiSeparator.ReplaceAllString(safe, "-")
	return strings.Trim(safe, "-")
}

// MakeSourceDirName produces the numbered output folder name: "0001_filename"
func MakeSourceDirName(counter int, sourceFilename string) string {
	return fmt.Sprintf("%04d_%s", counter, SanitizeName(sourceFilename))
}

// Timestamp returns a formatted timestamp for logging.
func Timestamp() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

func TimestampForFilename() string {
	return time.Now().Format("2006-01-02_150405")
}
