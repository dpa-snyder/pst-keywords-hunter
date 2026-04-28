package scanner

import (
	"bufio"
	"os"
)

// ExtractHeader reads an EML file and returns everything up to the first
// blank line (the RFC-822 headers). Matches the original awk behaviour:
//
//	awk '/^$/ {exit} {print}'
func ExtractHeader(emlPath string) (string, error) {
	f, err := os.Open(emlPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var header string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}
		header += line + "\n"
	}
	return header, scanner.Err()
}
