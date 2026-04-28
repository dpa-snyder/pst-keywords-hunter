package scanner

import (
	"encoding/csv"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

var (
	keywordSpacePattern  = regexp.MustCompile(`\s+`)
	keywordUnsafePattern = regexp.MustCompile(`[^a-z0-9\-]+`)
	keywordHyphenPattern = regexp.MustCompile(`-+`)
)

func CanonicalKeyword(term string) string {
	trimmed := strings.TrimSpace(term)
	if trimmed == "" {
		return ""
	}
	return strings.ToLower(keywordSpacePattern.ReplaceAllString(trimmed, " "))
}

// TermToDirname normalizes a keyword to a safe directory name.
func TermToDirname(term string) string {
	canonical := CanonicalKeyword(term)
	canonical = strings.ReplaceAll(canonical, " ", "-")
	canonical = keywordUnsafePattern.ReplaceAllString(canonical, "-")
	canonical = keywordHyphenPattern.ReplaceAllString(canonical, "-")
	return strings.Trim(canonical, "-")
}

func ParseInlineKeywords(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	reader := csv.NewReader(strings.NewReader(value))
	reader.FieldsPerRecord = -1
	if records, err := reader.ReadAll(); err == nil {
		var parsed []string
		for _, record := range records {
			for _, term := range record {
				if cleaned := cleanInlineKeyword(term); cleaned != "" {
					parsed = append(parsed, cleaned)
				}
			}
		}
		return parsed
	}

	var parsed []string
	for _, term := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	}) {
		if cleaned := cleanInlineKeyword(term); cleaned != "" {
			parsed = append(parsed, cleaned)
		}
	}
	return parsed
}

func cleanInlineKeyword(term string) string {
	term = strings.TrimSpace(term)
	term = strings.Trim(term, "\"'")
	term = strings.TrimSpace(term)
	return term
}

func MergeKeywordLists(lists ...[]string) []string {
	var merged []string
	seen := make(map[string]bool)
	for _, list := range lists {
		for _, term := range list {
			canonical := CanonicalKeyword(term)
			if canonical == "" || seen[canonical] {
				continue
			}
			seen[canonical] = true
			merged = append(merged, strings.TrimSpace(term))
		}
	}
	return merged
}

func FindKeywordConflicts(terms []string) []ConflictGroup {
	grouped := make(map[string][]string)
	for _, term := range terms {
		dir := TermToDirname(term)
		if dir == "" {
			continue
		}
		grouped[dir] = append(grouped[dir], term)
	}

	var conflicts []ConflictGroup
	for normalized, options := range grouped {
		if len(options) < 2 {
			continue
		}
		slices.Sort(options)
		conflicts = append(conflicts, ConflictGroup{
			Normalized: normalized,
			Options:    options,
		})
	}

	slices.SortFunc(conflicts, func(a, b ConflictGroup) int {
		return strings.Compare(a.Normalized, b.Normalized)
	})
	return conflicts
}

func ResolveKeywordConflict(terms []string, conflict ConflictGroup, keep string) ([]string, []RejectedKeyword, error) {
	valid := false
	for _, option := range conflict.Options {
		if option == keep {
			valid = true
			break
		}
	}
	if !valid {
		return nil, nil, fmt.Errorf("keep term %q not present in conflict group", keep)
	}

	var resolved []string
	var rejected []RejectedKeyword
	for _, term := range terms {
		if TermToDirname(term) != conflict.Normalized {
			resolved = append(resolved, term)
			continue
		}
		if term == keep {
			resolved = append(resolved, term)
			continue
		}
		rejected = append(rejected, RejectedKeyword{
			Requested:  term,
			Normalized: conflict.Normalized,
			Kept:       keep,
			Reason:     "duplicate normalized keyword",
		})
	}
	return resolved, rejected, nil
}
