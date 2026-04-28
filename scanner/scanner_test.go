package scanner

import (
	"encoding/csv"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func TestMergeKeywordListsAndConflicts(t *testing.T) {
	merged := MergeKeywordLists(
		[]string{"Harbor", " harbor ", "Project Alpha"},
		[]string{"project-alpha", "Juniper"},
	)

	expected := []string{"Harbor", "Project Alpha", "project-alpha", "Juniper"}
	if len(merged) != len(expected) {
		t.Fatalf("expected %d merged terms, got %d: %#v", len(expected), len(merged), merged)
	}
	for i := range expected {
		if merged[i] != expected[i] {
			t.Fatalf("expected merged[%d] = %q, got %q", i, expected[i], merged[i])
		}
	}

	conflicts := FindKeywordConflicts(merged)
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict group, got %d", len(conflicts))
	}
	if conflicts[0].Normalized != "project-alpha" {
		t.Fatalf("expected normalized conflict to be project-alpha, got %q", conflicts[0].Normalized)
	}
}

func TestParseInlineKeywordsTrimsQuotedCommaSeparatedTerms(t *testing.T) {
	parsed := ParseInlineKeywords(`"alex stone", "jordan reed"`)
	expected := []string{"alex stone", "jordan reed"}
	if len(parsed) != len(expected) {
		t.Fatalf("expected %d parsed terms, got %d: %#v", len(expected), len(parsed), parsed)
	}
	for i := range expected {
		if parsed[i] != expected[i] {
			t.Fatalf("expected parsed[%d] = %q, got %q", i, expected[i], parsed[i])
		}
	}
}

func TestLoadKeywordsFileTrimsQuotedLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keywords.txt")
	content := "# comment\n\"alex stone\"\n 'jordan reed' \n\ninfrastructure\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadKeywordsFile(path)
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{"alex stone", "jordan reed", "infrastructure"}
	if len(loaded) != len(expected) {
		t.Fatalf("expected %d terms, got %d: %#v", len(expected), len(loaded), loaded)
	}
	for i := range expected {
		if loaded[i] != expected[i] {
			t.Fatalf("expected loaded[%d] = %q, got %q", i, expected[i], loaded[i])
		}
	}
}

func TestLoadKeywordsFileParsesCsvLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keywords.txt")
	content := `"Northwind","AgencyX","ExampleCorp","Example Corp"` + "\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadKeywordsFile(path)
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{"Northwind", "AgencyX", "ExampleCorp", "Example Corp"}
	if len(loaded) != len(expected) {
		t.Fatalf("expected %d terms, got %d: %#v", len(expected), len(loaded), loaded)
	}
	for i := range expected {
		if loaded[i] != expected[i] {
			t.Fatalf("expected loaded[%d] = %q, got %q", i, expected[i], loaded[i])
		}
	}
}

func TestValidateRootsRejectsNestedPaths(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	outputInsideSource := filepath.Join(source, "output")
	outputParent := filepath.Join(root, "output")

	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outputInsideSource, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outputParent, 0755); err != nil {
		t.Fatal(err)
	}

	if err := ValidateRoots(source, outputInsideSource); err == nil {
		t.Fatalf("expected nested output directory to be rejected")
	}
	if err := ValidateRoots(source, outputParent); err != nil {
		t.Fatalf("expected sibling output directory to be allowed, got %v", err)
	}
}

func TestDateInRangeIsInclusive(t *testing.T) {
	start, err := ParseDateInput("2024-01-01")
	if err != nil {
		t.Fatal(err)
	}
	end, err := ParseDateInput("2024-01-31")
	if err != nil {
		t.Fatal(err)
	}

	emlDateTests := []struct {
		date string
		want bool
	}{
		{"Mon, 01 Jan 2024 09:00:00 -0500", true},
		{"Wed, 31 Jan 2024 18:30:00 -0500", true},
		{"Sun, 31 Dec 2023 23:59:59 -0500", false},
		{"Thu, 01 Feb 2024 00:00:00 -0500", false},
	}
	for _, tc := range emlDateTests {
		parsed, err := mailParseDate(tc.date)
		if err != nil {
			t.Fatal(err)
		}
		if got := DateInRange(parsed, start, end); got != tc.want {
			t.Fatalf("DateInRange(%q) = %v, want %v", tc.date, got, tc.want)
		}
	}
}

func TestProcessEMLDirPreservesRelativePathsAndAvoidsOverwrite(t *testing.T) {
	sourceRoot := t.TempDir()
	outputRoot := t.TempDir()
	runDir := filepath.Join(outputRoot, "2026-03-23_120000")
	if err := os.MkdirAll(runDir, 0755); err != nil {
		t.Fatal(err)
	}
	emlDir := t.TempDir()
	for _, rel := range []string{"Inbox/123.eml", "Sent/123.eml"} {
		fullPath := filepath.Join(emlDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatal(err)
		}
		content := "From: test@example.com\nDate: Tue, 02 Jan 2024 10:00:00 -0500\nSubject: Test\n\nHarbor appears here.\n"
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	logger, err := NewLogger(runDir)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()

	cfg := Config{
		SourceDir: sourceRoot,
		OutputDir: outputRoot,
		Terms:     []string{"Harbor"},
	}
	artifacts := &runArtifacts{
		Timestamp: "2026-03-23_120000",
		RunDir:    runDir,
	}
	summary := &RunSummary{
		RunTimestamp:    artifacts.Timestamp,
		KeywordHits:     map[string]int{"Harbor": 0},
		UnknownDateHits: map[string]int{"Harbor": 0},
		HitsBySource:    map[string]int{},
		HitsByType:      map[string]int{},
	}
	sf := sourceFile{
		path:     filepath.Join(sourceRoot, "archive.pst"),
		relPath:  "archive.pst",
		fileType: TypePST,
	}
	sourceDirName := MakeSourceDirName(1, "archive.pst")
	sourceOutDir := filepath.Join(runDir, sourceDirName)
	if err := os.MkdirAll(sourceOutDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := processEMLDir(cfg, artifacts, summary, logger, nil, sf, sourceDirName, sourceOutDir, emlDir, 1, 1); err != nil {
		t.Fatal(err)
	}

	first := filepath.Join(sourceOutDir, "harbor", "inbox", "123.eml")
	second := filepath.Join(sourceOutDir, "harbor", "sent", "123.eml")
	if _, err := os.Stat(first); err != nil {
		t.Fatalf("expected first exported file at %s: %v", first, err)
	}
	if _, err := os.Stat(second); err != nil {
		t.Fatalf("expected second exported file at %s: %v", second, err)
	}
	if len(summary.ManifestRows) != 2 {
		t.Fatalf("expected 2 manifest rows, got %d", len(summary.ManifestRows))
	}
	if summary.ManifestRows[0].HitLocations != "body" || summary.ManifestRows[1].HitLocations != "body" {
		t.Fatalf("expected body hit locations, got %#v", []string{summary.ManifestRows[0].HitLocations, summary.ManifestRows[1].HitLocations})
	}
}

func TestProcessEMLDirDoesNotCreateSourceDirWhenNoHits(t *testing.T) {
	sourceRoot := t.TempDir()
	outputRoot := t.TempDir()
	runDir := filepath.Join(outputRoot, "2026-03-23_120000")
	if err := os.MkdirAll(runDir, 0755); err != nil {
		t.Fatal(err)
	}
	emlDir := t.TempDir()
	emlPath := filepath.Join(emlDir, "single.eml")
	content := "From: test@example.com\nDate: Tue, 02 Jan 2024 10:00:00 -0500\nSubject: Test\n\nNo matching term here.\n"
	if err := os.WriteFile(emlPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	logger, err := NewLogger(runDir)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()

	cfg := Config{
		SourceDir: sourceRoot,
		OutputDir: outputRoot,
		Terms:     []string{"Harbor"},
	}
	artifacts := &runArtifacts{
		Timestamp: "2026-03-23_120000",
		RunDir:    runDir,
	}
	summary := &RunSummary{
		RunTimestamp: "2026-03-23_120000",
		RunDir:       filepath.Base(runDir),
		KeywordHits:  make(map[string]int),
		HitsBySource: make(map[string]int),
		HitsByType:   make(map[string]int),
	}

	sourceDirName := MakeSourceDirName(1, "archive.pst")
	sourceOutDir := filepath.Join(runDir, sourceDirName)
	sf := sourceFile{
		path:     filepath.Join(sourceRoot, "archive.pst"),
		relPath:  "archive.pst",
		fileType: TypePST,
	}

	if err := processEMLDir(cfg, artifacts, summary, logger, nil, sf, sourceDirName, sourceOutDir, emlDir, 1, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sourceOutDir); !os.IsNotExist(err) {
		t.Fatalf("expected no source output directory for a no-hit source, got err=%v", err)
	}
	if got := summary.KeywordHits["Harbor"]; got != 0 {
		t.Fatalf("expected zero hits, got %d", got)
	}
}

func TestRunWithSummaryWritesReviewerAndTechnicalArtifacts(t *testing.T) {
	sourceRoot := t.TempDir()
	outputRoot := t.TempDir()

	bodyHit := "From: body@example.com\nDate: Tue, 02 Jan 2024 10:00:00 -0500\nSubject: Update\n\nHarbor appears in the body.\n"
	headerHit := "From: harbor@example.com\nDate: Tue, 02 Jan 2024 11:00:00 -0500\nSubject: Update\n\nNo body hit.\n"
	if err := os.WriteFile(filepath.Join(sourceRoot, "body-hit.eml"), []byte(bodyHit), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "header-hit.eml"), []byte(headerHit), 0644); err != nil {
		t.Fatal(err)
	}

	summary, err := RunWithSummary(Config{
		SourceDir: sourceRoot,
		OutputDir: outputRoot,
		Terms:     []string{"Harbor"},
		EnabledTypes: map[FileType]bool{
			TypeEML: true,
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(outputRoot, summary.ManifestPath)); err != nil {
		t.Fatalf("expected technical manifest: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputRoot, summary.ReviewManifestPath)); err != nil {
		t.Fatalf("expected reviewer manifest: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputRoot, summary.ManifestWorkbookPath)); err != nil {
		t.Fatalf("expected technical workbook: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputRoot, summary.ReviewWorkbookPath)); err != nil {
		t.Fatalf("expected reviewer workbook: %v", err)
	}

	reviewManifest, err := os.Open(filepath.Join(outputRoot, summary.ReviewManifestPath))
	if err != nil {
		t.Fatal(err)
	}
	defer reviewManifest.Close()
	reviewRecords, err := csv.NewReader(reviewManifest).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(reviewRecords) < 3 {
		t.Fatalf("expected reviewer rows, got %d", len(reviewRecords))
	}
	if got := reviewRecords[0][0]; got != "base_name" {
		t.Fatalf("unexpected reviewer manifest header %q", got)
	}
	reviewColumns := headerIndexMap(reviewRecords[0])
	if got := reviewRecords[1][reviewColumns["base_name"]]; got != "body-hit.eml" {
		t.Fatalf("expected body hit first in reviewer manifest, got %q", got)
	}
	if got := reviewRecords[1][reviewColumns["hit_locations"]]; got != "Body" {
		t.Fatalf("expected human hit locations, got %q", got)
	}
	if got := reviewRecords[1][reviewColumns["status"]]; got != "exported" {
		t.Fatalf("expected human status label, got %q", got)
	}
	if got := reviewRecords[1][reviewColumns["size_human"]]; got == "" {
		t.Fatalf("expected reviewer human size")
	}

	technicalManifest, err := os.Open(filepath.Join(outputRoot, summary.ManifestPath))
	if err != nil {
		t.Fatal(err)
	}
	defer technicalManifest.Close()
	technicalRecords, err := csv.NewReader(technicalManifest).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(technicalRecords) < 3 {
		t.Fatalf("expected technical rows, got %d", len(technicalRecords))
	}
	if got := technicalRecords[0][0]; got != "base_name" {
		t.Fatalf("unexpected technical manifest header %q", got)
	}
	technicalColumns := headerIndexMap(technicalRecords[0])
	if got := technicalRecords[1][technicalColumns["output_eml_directory_path"]]; got == "" {
		t.Fatalf("expected technical output eml directory path")
	}

	reviewBook, err := excelize.OpenFile(filepath.Join(outputRoot, summary.ReviewWorkbookPath))
	if err != nil {
		t.Fatalf("expected reviewer workbook to open: %v", err)
	}
	defer reviewBook.Close()
	reviewRows, err := reviewBook.GetRows("Reviewer Manifest")
	if err != nil {
		t.Fatalf("expected reviewer workbook rows: %v", err)
	}
	if len(reviewRows) < 3 {
		t.Fatalf("expected reviewer workbook rows, got %d", len(reviewRows))
	}
	if got := reviewRows[0][0]; got != "base_name" {
		t.Fatalf("unexpected reviewer workbook header %q", got)
	}

	technicalBook, err := excelize.OpenFile(filepath.Join(outputRoot, summary.ManifestWorkbookPath))
	if err != nil {
		t.Fatalf("expected technical workbook to open: %v", err)
	}
	defer technicalBook.Close()
	technicalRows, err := technicalBook.GetRows("Technical Manifest")
	if err != nil {
		t.Fatalf("expected technical workbook rows: %v", err)
	}
	if len(technicalRows) < 3 {
		t.Fatalf("expected technical workbook rows, got %d", len(technicalRows))
	}

	reportData, err := os.ReadFile(filepath.Join(outputRoot, summary.ReportPath))
	if err != nil {
		t.Fatal(err)
	}
	reportText := string(reportData)
	if strings.Contains(reportText, "## Hits By Source Container") {
		t.Fatalf("expected slimmed report without per-source section")
	}
	if !strings.Contains(reportText, "Reviewer spreadsheet") {
		t.Fatalf("expected report to point to reviewer spreadsheet")
	}
}

func mailParseDate(value string) (time.Time, error) {
	return mail.ParseDate(value)
}

func headerIndexMap(headers []string) map[string]int {
	indexes := make(map[string]int, len(headers))
	for i, header := range headers {
		indexes[header] = i
	}
	return indexes
}

func TestFindKeywordLocationsBytesReturnsHeaderSubjectAndBody(t *testing.T) {
	data := []byte("From: test@example.com\nSubject: Harbor update\nX-Custom: harbor marker\n\nThe body mentions Harbor too.\n")
	locations := FindKeywordLocationsBytes(data, "Harbor")
	got := strings.Join(locations, "|")
	if got != "header|subject|body" {
		t.Fatalf("expected header|subject|body, got %q", got)
	}
}
