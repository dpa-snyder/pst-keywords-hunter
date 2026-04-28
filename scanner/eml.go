package scanner

import (
	"os"
	"path/filepath"
	"strings"
)

// FindEMLFiles recursively finds all .eml files under a directory.
func FindEMLFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.EqualFold(filepath.Ext(path), ".eml") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// ProcessEMLDir searches all .eml files in a directory for keyword matches
// and writes hits to the output structure. This is the core inner loop
// shared by all source types (PST extraction dir, loose EMLs, converted MSGs, etc.).
func ProcessEMLDir(
	emlDir string,
	terms []string,
	sourceOutDir string,
	logger *Logger,
	events chan<- Event,
	fileNum, totalFiles int,
	sourceFile string,
	sourceType FileType,
) {
	emlFiles, err := FindEMLFiles(emlDir)
	if err != nil {
		msg := "  Error finding EML files: " + err.Error()
		logger.Log(msg)
		if events != nil {
			events <- Event{Type: EventError, Message: msg}
		}
		return
	}

	if len(emlFiles) == 0 {
		msg := "  No EML files found in " + emlDir
		logger.Log(msg)
		return
	}

	for _, term := range terms {
		msg := "Searching for term: " + term
		logger.Log(msg)
		if events != nil {
			events <- Event{
				Type: EventSearching, Term: term,
				SourceFile: sourceFile, FileNum: fileNum, TotalFiles: totalFiles,
				Message: msg,
			}
		}

		for _, emlPath := range emlFiles {
			matched, err := ContainsKeyword(emlPath, term)
			if err != nil {
				logger.Log("  Warning: error reading " + emlPath + ": " + err.Error())
				continue
			}
			if matched {
				termDir := filepath.Join(sourceOutDir, TermToDirname(term))
				os.MkdirAll(termDir, 0755)

				stem := strings.TrimSuffix(filepath.Base(emlPath), filepath.Ext(emlPath))

				// Write header file
				header, err := ExtractHeader(emlPath)
				if err == nil {
					headerPath := filepath.Join(termDir, stem+"_header.txt")
					os.WriteFile(headerPath, []byte(header), 0644)
				}

				// Copy full EML
				data, err := os.ReadFile(emlPath)
				if err == nil {
					destPath := filepath.Join(termDir, stem+".eml")
					os.WriteFile(destPath, data, 0644)
				}

				matchMsg := "Found a match for term: " + term + " in " + emlPath
				logger.Log(matchMsg)
				if events != nil {
					events <- Event{
						Type: EventMatch, Term: term,
						SourceFile: sourceFile, OutputDir: sourceOutDir,
						FileNum: fileNum, TotalFiles: totalFiles,
						Message: matchMsg,
					}
				}
			}
		}

		doneMsg := "Finished searching for term: " + term
		logger.Log(doneMsg)
		if events != nil {
			events <- Event{
				Type: EventSearchDone, Term: term,
				SourceFile: sourceFile, FileNum: fileNum, TotalFiles: totalFiles,
				Message: doneMsg,
			}
		}
	}
}
