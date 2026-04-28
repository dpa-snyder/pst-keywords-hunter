package scanner

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// SplitMBOX splits an mbox file into individual .eml files in outDir.
// The mbox format uses "From " lines (with a space) as message delimiters.
// Returns the number of messages extracted.
func SplitMBOX(mboxPath string, outDir string) (int, error) {
	f, err := os.Open(mboxPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open mbox: %w", err)
	}
	defer f.Close()

	r := bufio.NewReaderSize(f, 64*1024)

	var (
		msgCount int
		current  *os.File
	)

	trimTrailingSeparatorNewline := func(f *os.File) {
		info, err := f.Stat()
		if err != nil || info.Size() < 2 {
			return
		}

		size := info.Size()
		tailLen := int64(4)
		if size < tailLen {
			tailLen = size
		}

		buf := make([]byte, tailLen)
		if _, err := f.ReadAt(buf, size-tailLen); err != nil && err != io.EOF {
			return
		}

		tail := string(buf)
		switch {
		case strings.HasSuffix(tail, "\r\n\r\n"):
			_ = f.Truncate(size - 2)
		case strings.HasSuffix(tail, "\n\n"):
			_ = f.Truncate(size - 1)
		}
	}

	closeCurrent := func() {
		if current != nil {
			trimTrailingSeparatorNewline(current)
			current.Close()
			current = nil
		}
	}

	readLine := func() (string, error) {
		var line strings.Builder
		for {
			part, err := r.ReadString('\n')
			if part != "" {
				line.WriteString(part)
				if strings.HasSuffix(part, "\n") {
					return line.String(), nil
				}
			}
			if err != nil {
				if err == io.EOF && line.Len() > 0 {
					return line.String(), nil
				}
				return "", err
			}
		}
	}

	for {
		line, err := readLine()
		if err == io.EOF {
			break
		}
		if err != nil {
			return msgCount, fmt.Errorf("error reading mbox: %w", err)
		}

		lineContent := strings.TrimRight(line, "\r\n")

		// "From " at the start of a line marks a new message boundary
		if strings.HasPrefix(lineContent, "From ") {
			closeCurrent()
			msgCount++

			emlPath := filepath.Join(outDir, fmt.Sprintf("message_%04d.eml", msgCount))
			current, err = os.OpenFile(emlPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
			if err != nil {
				return msgCount - 1, fmt.Errorf("failed to create EML file: %w", err)
			}
			// Don't write the "From " separator line to the EML
			continue
		}

		if current != nil {
			// Undo mbox "From " escaping: ">From " -> "From " in body
			if strings.HasPrefix(lineContent, ">From ") {
				line = line[1:]
			}
			if _, err := current.WriteString(line); err != nil {
				return msgCount, fmt.Errorf("failed to write EML file: %w", err)
			}
		}
	}

	closeCurrent()

	return msgCount, nil
}
