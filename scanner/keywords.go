package scanner

import (
	"bufio"
	"bytes"
	"net/mail"
	"os"
	"strings"
)

// ContainsKeyword performs a case-insensitive fixed-string search on a file.
// Equivalent to: grep -q -i -F "$term" "$file"
// Short-circuits on first match.
func ContainsKeyword(filePath string, term string) (bool, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	needle := strings.ToLower(term)
	scanner := bufio.NewScanner(f)
	// Handle very long lines (some EML files have base64 blocks)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 20*1024*1024)

	for scanner.Scan() {
		if strings.Contains(strings.ToLower(scanner.Text()), needle) {
			return true, nil
		}
	}
	return false, scanner.Err()
}

// ContainsKeywordBytes performs a case-insensitive search on raw bytes.
// Used for in-memory content (e.g. converted MSG files).
func ContainsKeywordBytes(data []byte, term string) bool {
	needle := strings.ToLower(term)
	content := strings.ToLower(string(data))
	return strings.Contains(content, needle)
}

// FindKeywordLocations reports which message sections contain the term.
// Locations are limited to header, subject, and body for downstream review logs.
func FindKeywordLocations(filePath string, term string) ([]string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return FindKeywordLocationsBytes(data, term), nil
}

// FindKeywordLocationsBytes reports which message sections contain the term.
func FindKeywordLocationsBytes(data []byte, term string) []string {
	needle := strings.ToLower(term)
	if needle == "" {
		return nil
	}

	header, body := splitMessageSections(data)
	locations := make([]string, 0, 3)
	if strings.Contains(strings.ToLower(header), needle) {
		locations = append(locations, "header")
	}
	if subject := parseMessageSubject(data); subject != "" && strings.Contains(strings.ToLower(subject), needle) {
		locations = append(locations, "subject")
	}
	if strings.Contains(strings.ToLower(body), needle) {
		locations = append(locations, "body")
	}
	return locations
}

func splitMessageSections(data []byte) (header string, body string) {
	switch {
	case bytes.Contains(data, []byte("\r\n\r\n")):
		parts := bytes.SplitN(data, []byte("\r\n\r\n"), 2)
		return string(parts[0]), string(parts[1])
	case bytes.Contains(data, []byte("\n\n")):
		parts := bytes.SplitN(data, []byte("\n\n"), 2)
		return string(parts[0]), string(parts[1])
	default:
		return string(data), ""
	}
}

func parseMessageSubject(data []byte) string {
	msg, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		return ""
	}
	return msg.Header.Get("Subject")
}

// LoadKeywordsFile reads keywords from a file. Each non-comment line may be a
// single term or a CSV-style list of terms.
func LoadKeywordsFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var terms []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		terms = append(terms, ParseInlineKeywords(line)...)
	}
	return terms, scanner.Err()
}
