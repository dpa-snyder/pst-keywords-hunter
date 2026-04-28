package scanner

import (
	"fmt"
	"os"
	"path/filepath"
)

// CountMatchesInEMLDir counts matching EML messages for each term under emlDir.
// It mirrors full-scan behavior, but does not write any output files.
func CountMatchesInEMLDir(emlDir string, terms []string) (map[string]int, error) {
	counts := make(map[string]int, len(terms))
	for _, term := range terms {
		counts[term] = 0
	}

	emlFiles, err := FindEMLFiles(emlDir)
	if err != nil {
		return counts, err
	}

	for _, term := range terms {
		for _, emlPath := range emlFiles {
			matched, err := ContainsKeyword(emlPath, term)
			if err != nil {
				continue
			}
			if matched {
				counts[term]++
			}
		}
	}

	return counts, nil
}

// CountKeywordMatches performs a dry-run keyword sweep without exporting results.
// Counts represent how many EML messages would be exported under each term.
func CountKeywordMatches(cfg Config, discovered map[FileType][]string) (map[string]int, int, error) {
	totalCounts := make(map[string]int, len(cfg.Terms))
	for _, term := range cfg.Terms {
		totalCounts[term] = 0
	}

	if len(cfg.Terms) == 0 {
		return totalCounts, 0, nil
	}

	type sourceFile struct {
		path     string
		fileType FileType
	}

	var allFiles []sourceFile
	for _, ft := range []FileType{TypePST, TypeOST, TypeMBOX, TypeEML, TypeMSG} {
		for _, path := range discovered[ft] {
			allFiles = append(allFiles, sourceFile{path: path, fileType: ft})
		}
	}

	filesScanned := 0
	for _, sf := range allFiles {
		switch sf.fileType {
		case TypePST, TypeOST:
			if !HasReadPST() {
				continue
			}

			tmpDir, err := os.MkdirTemp("", "kh-dryrun-pst-*")
			if err != nil {
				return totalCounts, filesScanned, err
			}

			err = ExtractPST(sf.path, tmpDir, nil)
			if err == nil {
				filesScanned++
				counts, countErr := CountMatchesInEMLDir(tmpDir, cfg.Terms)
				if countErr == nil {
					mergeTermCounts(totalCounts, counts)
				}
			}
			_ = os.RemoveAll(tmpDir)

		case TypeEML:
			tmpDir, err := os.MkdirTemp("", "kh-dryrun-eml-*")
			if err != nil {
				return totalCounts, filesScanned, err
			}

			data, err := os.ReadFile(sf.path)
			if err == nil {
				destPath := filepath.Join(tmpDir, filepath.Base(sf.path))
				if err = os.WriteFile(destPath, data, 0644); err == nil {
					filesScanned++
					counts, countErr := CountMatchesInEMLDir(tmpDir, cfg.Terms)
					if countErr == nil {
						mergeTermCounts(totalCounts, counts)
					}
				}
			}
			_ = os.RemoveAll(tmpDir)

		case TypeMSG:
			tmpDir, err := os.MkdirTemp("", "kh-dryrun-msg-*")
			if err != nil {
				return totalCounts, filesScanned, err
			}

			if _, err = ConvertMSGToEML(sf.path, tmpDir); err == nil {
				filesScanned++
				counts, countErr := CountMatchesInEMLDir(tmpDir, cfg.Terms)
				if countErr == nil {
					mergeTermCounts(totalCounts, counts)
				}
			}
			_ = os.RemoveAll(tmpDir)

		case TypeMBOX:
			tmpDir, err := os.MkdirTemp("", "kh-dryrun-mbox-*")
			if err != nil {
				return totalCounts, filesScanned, err
			}

			if _, err = SplitMBOX(sf.path, tmpDir); err == nil {
				filesScanned++
				counts, countErr := CountMatchesInEMLDir(tmpDir, cfg.Terms)
				if countErr == nil {
					mergeTermCounts(totalCounts, counts)
				}
			}
			_ = os.RemoveAll(tmpDir)

		default:
			return totalCounts, filesScanned, fmt.Errorf("unsupported file type: %s", sf.fileType)
		}
	}

	return totalCounts, filesScanned, nil
}

func mergeTermCounts(dst, src map[string]int) {
	for term, count := range src {
		dst[term] += count
	}
}
