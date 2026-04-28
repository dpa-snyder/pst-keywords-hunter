package filescanner

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type DateStatus string

const (
	DateStatusInRange          DateStatus = "in_range"
	DateStatusUnknown          DateStatus = "unknown"
	DateStatusExcludedPreRange DateStatus = "excluded_pre_range"
)

type dateMetadata struct {
	FilesystemCreated  *time.Time
	FilesystemModified *time.Time
	EmbeddedCreated    *time.Time
	EmbeddedModified   *time.Time
	Selected           *time.Time
	SelectedSource     string
	Status             DateStatus
	StatusNote         string
}

func determineDateMetadata(file classifiedFile, startDate, endDate *time.Time) dateMetadata {
	if cached, ok := cachedDateMetadata(file); ok {
		classifyDocumentDate(&cached, startDate, endDate)
		return cached
	}

	metadata := collectDateMetadata(file, nil)
	classifyDocumentDate(&metadata, startDate, endDate)
	return metadata
}

func cachedDateMetadata(file classifiedFile) (dateMetadata, bool) {
	metadata := cloneDateMetadata(file.DateMetadata)
	if metadata.FilesystemCreated == nil &&
		metadata.FilesystemModified == nil &&
		metadata.EmbeddedCreated == nil &&
		metadata.EmbeddedModified == nil &&
		metadata.Selected == nil &&
		metadata.SelectedSource == "" {
		return dateMetadata{}, false
	}
	return metadata, true
}

func collectDateMetadata(file classifiedFile, info os.FileInfo) dateMetadata {
	metadata := dateMetadata{}
	if info == nil {
		statInfo, err := os.Stat(file.Path)
		if err == nil {
			info = statInfo
		} else {
			metadata.StatusNote = fmt.Sprintf("date metadata unavailable: %v", err)
		}
	}

	if info != nil {
		modified := info.ModTime()
		metadata.FilesystemModified = &modified
		if created, ok := lookupFilesystemBirthTime(file.Path, info); ok {
			metadata.FilesystemCreated = created
		}
	}

	embeddedCreated, embeddedModified := readEmbeddedDocumentDates(file)
	metadata.EmbeddedCreated = embeddedCreated
	metadata.EmbeddedModified = embeddedModified
	selectBestDocumentDate(&metadata)
	if metadata.Selected == nil {
		applyFolderYearFallback(&metadata, file.RelPath)
	}
	return metadata
}

func cloneDateMetadata(metadata dateMetadata) dateMetadata {
	clone := metadata
	clone.FilesystemCreated = cloneOptionalTime(metadata.FilesystemCreated)
	clone.FilesystemModified = cloneOptionalTime(metadata.FilesystemModified)
	clone.EmbeddedCreated = cloneOptionalTime(metadata.EmbeddedCreated)
	clone.EmbeddedModified = cloneOptionalTime(metadata.EmbeddedModified)
	clone.Selected = cloneOptionalTime(metadata.Selected)
	clone.Status = ""
	clone.StatusNote = ""
	return clone
}

func cloneOptionalTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func extractFolderYear(relPath string) (int, bool) {
	parts := strings.Split(filepath.ToSlash(relPath), "/")
	if len(parts) <= 1 {
		return 0, false
	}
	for i := len(parts) - 2; i >= 0; i-- {
		for _, token := range tokenizeYearCandidates(parts[i]) {
			if len(token) != 4 {
				continue
			}
			year, err := strconv.Atoi(token)
			if err != nil {
				continue
			}
			if year >= 1900 && year <= 2099 {
				return year, true
			}
		}
	}
	return 0, false
}

func tokenizeYearCandidates(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return (r < '0' || r > '9')
	})
}

func applyFolderYear(metadata *dateMetadata, year int, startDate, endDate *time.Time) {
	folderTime := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
	metadata.Selected = &folderTime
	metadata.SelectedSource = "folder_year"

	if startDate != nil && year < startDate.Year() {
		metadata.Status = DateStatusExcludedPreRange
		metadata.StatusNote = fmt.Sprintf("folder year %d is before the review range and was used as the fallback date signal", year)
		return
	}
	if endDate != nil && year > endDate.Year() {
		metadata.Status = DateStatusUnknown
		metadata.StatusNote = fmt.Sprintf("folder year %d is after the review range and is treated as unknown by policy", year)
		return
	}

	metadata.Status = DateStatusInRange
	metadata.StatusNote = fmt.Sprintf("folder year %d was used as the document date", year)
}

func applyFolderYearFallback(metadata *dateMetadata, relPath string) {
	if metadata.Selected != nil || metadata.SelectedSource != "" {
		return
	}
	year, ok := extractFolderYear(relPath)
	if !ok {
		return
	}
	applyFolderYear(metadata, year, nil, nil)
	if metadata.SelectedSource == "folder_year" {
		metadata.StatusNote = fmt.Sprintf("folder year %d was used as a fallback date signal", year)
	}
}

func selectBestDocumentDate(metadata *dateMetadata) {
	switch {
	case metadata.FilesystemCreated != nil:
		metadata.Selected = metadata.FilesystemCreated
		metadata.SelectedSource = "filesystem_created"
	case metadata.EmbeddedCreated != nil:
		metadata.Selected = metadata.EmbeddedCreated
		metadata.SelectedSource = "embedded_created"
	case metadata.FilesystemModified != nil:
		metadata.Selected = metadata.FilesystemModified
		metadata.SelectedSource = "filesystem_modified"
	case metadata.EmbeddedModified != nil:
		metadata.Selected = metadata.EmbeddedModified
		metadata.SelectedSource = "embedded_modified"
	}
}

func classifyDocumentDate(metadata *dateMetadata, startDate, endDate *time.Time) {
	if metadata.Selected == nil {
		if metadata.StatusNote == "" {
			metadata.StatusNote = "no usable date metadata was available"
		}
		metadata.Status = DateStatusUnknown
		return
	}

	selectedDay := metadata.Selected.Format("2006-01-02")
	if startDate != nil && selectedDay < startDate.Format("2006-01-02") {
		metadata.Status = DateStatusExcludedPreRange
		metadata.StatusNote = fmt.Sprintf("best available date is before the review range (%s)", selectedDay)
		return
	}
	if endDate != nil && selectedDay > endDate.Format("2006-01-02") {
		metadata.Status = DateStatusUnknown
		metadata.StatusNote = fmt.Sprintf("best available date is after the review range (%s) and is treated as unknown by policy", selectedDay)
		return
	}

	metadata.Status = DateStatusInRange
	if metadata.StatusNote == "" {
		metadata.StatusNote = fmt.Sprintf("best available date selected from %s", metadata.SelectedSource)
	}
}

func readEmbeddedDocumentDates(file classifiedFile) (*time.Time, *time.Time) {
	if file.SearchMethod != MethodOpenXML {
		return nil, nil
	}

	zr, err := zip.OpenReader(file.Path)
	if err != nil {
		return nil, nil
	}
	defer zr.Close()

	for _, zf := range zr.File {
		if zf.Name != "docProps/core.xml" {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return nil, nil
		}
		defer rc.Close()

		var props struct {
			Created  string `xml:"http://purl.org/dc/terms/ created"`
			Modified string `xml:"http://purl.org/dc/terms/ modified"`
		}
		if err := xml.NewDecoder(rc).Decode(&props); err != nil {
			return nil, nil
		}
		return parseFlexibleTime(props.Created), parseFlexibleTime(props.Modified)
	}

	return nil, nil
}

func parseFlexibleTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return &parsed
		}
	}
	return nil
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(time.RFC3339)
}

func relativeDateNote(status DateStatus, source string) string {
	switch status {
	case DateStatusExcludedPreRange:
		return "excluded because the best available date is before the review range"
	case DateStatusUnknown:
		if source != "" {
			return fmt.Sprintf("treated as unknown using %s under the current date policy", source)
		}
		return "treated as unknown under the current date policy"
	default:
		if source != "" {
			return fmt.Sprintf("included using %s", source)
		}
		return ""
	}
}

func normalizeSourceLabel(sourceDir, relPath string) string {
	base := filepath.Base(sourceDir)
	return filepath.ToSlash(filepath.Join(base, relPath))
}
