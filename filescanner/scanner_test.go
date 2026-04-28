package filescanner

import (
	"archive/zip"
	"encoding/csv"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func TestPrescanCountsSearchableAndIgnoredFiles(t *testing.T) {
	sourceDir := t.TempDir()

	mustWriteFile(t, filepath.Join(sourceDir, "docs", "hit.txt"), "harbor appears in content")
	mustWriteFile(t, filepath.Join(sourceDir, "docs", "notes.docx"), "placeholder")
	mustWriteFile(t, filepath.Join(sourceDir, "docs", "~$notes.docx"), "office lock file")
	mustWriteFile(t, filepath.Join(sourceDir, "mail", "archive.eml"), "ignored email")
	mustWriteFile(t, filepath.Join(sourceDir, "misc", "blob.bin"), "binary-ish")

	prescan, err := Prescan(sourceDir)
	if err != nil {
		t.Fatal(err)
	}

	if prescan.FilesDiscovered != 4 {
		t.Fatalf("expected 4 discovered files after skipping office lock file, got %d", prescan.FilesDiscovered)
	}
	if prescan.IgnoredEmailFiles != 1 {
		t.Fatalf("expected 1 ignored email file, got %d", prescan.IgnoredEmailFiles)
	}
	if prescan.ContentSearchableFiles != 2 {
		t.Fatalf("expected 2 content-searchable files, got %d", prescan.ContentSearchableFiles)
	}
	if prescan.FilenameOnlyFiles != 1 {
		t.Fatalf("expected 1 filename-only file, got %d", prescan.FilenameOnlyFiles)
	}
}

func TestRunWithSummaryCopiesMatchedFileOnce(t *testing.T) {
	sourceDir := t.TempDir()
	outputDir := t.TempDir()

	mustWriteFile(t, filepath.Join(sourceDir, "docs", "meeting-notes.txt"), "harbor appears in the body. harbor appears again.")
	mustWriteFile(t, filepath.Join(sourceDir, "harbor-folder", "unrelated.bin"), "no content extraction needed")
	mustWriteFile(t, filepath.Join(sourceDir, "harbor-searchable", "unrelated.txt"), "no relevant term here")
	mustWriteFile(t, filepath.Join(sourceDir, "misc", "harbor-filename.bin"), "no content extraction needed")
	mustWriteFile(t, filepath.Join(sourceDir, "misc", "other.txt"), "no relevant term here")

	summary, err := RunWithSummary(Config{
		SourceDir: sourceDir,
		OutputDir: outputDir,
		Terms:     []string{"harbor"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if summary.MatchedFiles != 3 {
		t.Fatalf("expected 3 matched files, got %d", summary.MatchedFiles)
	}
	if summary.CopiedFiles != 3 {
		t.Fatalf("expected 3 copied files, got %d", summary.CopiedFiles)
	}
	if got := summary.FilenameHitsByKeyword["harbor"]; got != 1 {
		t.Fatalf("expected 1 filename hit, got %d", got)
	}
	if got := summary.FolderHitsByKeyword["harbor"]; got != 1 {
		t.Fatalf("expected 1 folder path hit, got %d", got)
	}
	if got := summary.ContentHitsByKeyword["harbor"]; got != 2 {
		t.Fatalf("expected 2 content hits, got %d", got)
	}
	if len(summary.ManifestRows) != 3 {
		t.Fatalf("expected 3 manifest rows, got %d", len(summary.ManifestRows))
	}
	reviewManifest, err := os.Open(summary.ReviewManifestPath)
	if err != nil {
		t.Fatalf("expected reviewer manifest at %s: %v", summary.ReviewManifestPath, err)
	}
	defer reviewManifest.Close()
	reviewRecords, err := csv.NewReader(reviewManifest).ReadAll()
	if err != nil {
		t.Fatalf("expected reviewer manifest rows: %v", err)
	}
	reviewHeader := reviewRecords[0]
	if len(reviewHeader) == 0 || reviewHeader[0] != "base_name" {
		t.Fatalf("unexpected reviewer manifest header: %v", reviewHeader)
	}
	reviewColumns := headerIndexMap(reviewHeader)
	for _, column := range reviewHeader {
		if column == "filesystem_created" || column == "embedded_created" || column == "date_note" {
			t.Fatalf("reviewer manifest should omit technical-only column %q", column)
		}
	}
	if got := reviewRecords[1][reviewColumns["base_name"]]; got != "meeting-notes.txt" {
		t.Fatalf("expected reviewer manifest base name first, got %q", got)
	}
	if got := reviewRecords[1][reviewColumns["source_directory_path"]]; got != "docs" {
		t.Fatalf("expected reviewer manifest source directory path, got %q", got)
	}
	if got := reviewRecords[1][reviewColumns["copied_directory_path"]]; got != filepath.ToSlash(filepath.Join(summary.RunDir, "matched_files", "docs")) {
		t.Fatalf("expected reviewer manifest copied directory path, got %q", got)
	}
	if got := reviewRecords[1][reviewColumns["document_date_source"]]; got != "file created time" {
		t.Fatalf("expected human-readable review date source, got %q", got)
	}
	if got := reviewRecords[1][reviewColumns["content_status"]]; got != "content searched" {
		t.Fatalf("expected human-readable review content status, got %q", got)
	}
	if got := reviewRecords[1][reviewColumns["size_human"]]; got == "" {
		t.Fatalf("expected reviewer manifest human size value")
	}
	if _, err := os.Stat(summary.ReviewWorkbookPath); err != nil {
		t.Fatalf("expected reviewer workbook at %s: %v", summary.ReviewWorkbookPath, err)
	}
	if _, err := os.Stat(summary.ManifestWorkbookPath); err != nil {
		t.Fatalf("expected technical workbook at %s: %v", summary.ManifestWorkbookPath, err)
	}
	reviewBook, err := excelize.OpenFile(summary.ReviewWorkbookPath)
	if err != nil {
		t.Fatalf("expected reviewer workbook to open: %v", err)
	}
	defer reviewBook.Close()
	reviewSheetRows, err := reviewBook.GetRows("Reviewer Manifest")
	if err != nil {
		t.Fatalf("expected reviewer workbook rows: %v", err)
	}
	if len(reviewSheetRows) < 2 {
		t.Fatalf("expected reviewer workbook data rows, got %d", len(reviewSheetRows))
	}
	if got := reviewSheetRows[0][0]; got != "base_name" {
		t.Fatalf("unexpected reviewer workbook header %q", got)
	}
	reviewSheetColumns := headerIndexMap(reviewSheetRows[0])
	if got := reviewSheetRows[1][reviewSheetColumns["base_name"]]; got != "meeting-notes.txt" {
		t.Fatalf("expected reviewer workbook base name first, got %q", got)
	}
	if got := reviewSheetRows[1][reviewSheetColumns["source_directory_path"]]; got != "docs" {
		t.Fatalf("expected reviewer workbook source directory path, got %q", got)
	}
	if got := reviewSheetRows[1][reviewSheetColumns["document_date_source"]]; got != "file created time" {
		t.Fatalf("expected reviewer workbook human date source, got %q", got)
	}
	if got := reviewSheetRows[1][reviewSheetColumns["content_status"]]; got != "content searched" {
		t.Fatalf("expected reviewer workbook human content status, got %q", got)
	}
	technicalBook, err := excelize.OpenFile(summary.ManifestWorkbookPath)
	if err != nil {
		t.Fatalf("expected technical workbook to open: %v", err)
	}
	defer technicalBook.Close()
	technicalRows, err := technicalBook.GetRows("Technical Manifest")
	if err != nil {
		t.Fatalf("expected technical workbook rows: %v", err)
	}
	if len(technicalRows) < 2 {
		t.Fatalf("expected technical workbook data rows, got %d", len(technicalRows))
	}
	if got := technicalRows[0][0]; got != "base_name" {
		t.Fatalf("unexpected technical workbook header %q", got)
	}
	technicalColumns := headerIndexMap(technicalRows[0])
	if got := technicalRows[1][technicalColumns["base_name"]]; got != "meeting-notes.txt" {
		t.Fatalf("expected technical workbook first data row to mirror manifest order, got %q", got)
	}
	if got := technicalRows[1][technicalColumns["source_directory_path"]]; got != "docs" {
		t.Fatalf("expected technical workbook source directory path, got %q", got)
	}
	if got := technicalRows[1][technicalColumns["size_human"]]; got == "" {
		t.Fatalf("expected technical workbook human size value")
	}
	var bodyMatchFound bool
	var folderMatchFound bool
	for _, row := range summary.ManifestRows {
		if row.CopiedFilePath == "" {
			t.Fatalf("expected copied file path for %s", row.SourceRelativePath)
		}
		target := filepath.Join(outputDir, row.CopiedFilePath)
		if _, err := os.Stat(target); err != nil {
			t.Fatalf("expected copied file at %s: %v", target, err)
		}
		if row.SourceRelativePath == "docs/meeting-notes.txt" {
			bodyMatchFound = true
			wantCopiedSuffix := filepath.ToSlash(filepath.Join(summary.RunDir, "matched_files", "docs", "meeting-notes.txt"))
			if row.CopiedFilePath != wantCopiedSuffix {
				t.Fatalf("expected copied file path to preserve source folders, got %q want %q", row.CopiedFilePath, wantCopiedSuffix)
			}
			if row.ContentHitCounts["harbor"] != 2 {
				t.Fatalf("expected body match count 2, got %d", row.ContentHitCounts["harbor"])
			}
			if row.ContentHitTotal != 2 {
				t.Fatalf("expected content hit total 2, got %d", row.ContentHitTotal)
			}
		}
		if row.SourceRelativePath == "harbor-folder" {
			folderMatchFound = true
			wantCopiedSuffix := filepath.ToSlash(filepath.Join(summary.RunDir, "matched_files", "harbor-folder"))
			if row.CopiedFilePath != wantCopiedSuffix {
				t.Fatalf("expected copied folder path to preserve source folders, got %q want %q", row.CopiedFilePath, wantCopiedSuffix)
			}
			if row.Extension != "[folder]" {
				t.Fatalf("expected folder match extension marker, got %q", row.Extension)
			}
			if row.FolderHitCounts["harbor"] != 1 {
				t.Fatalf("expected folder match count 1, got %d", row.FolderHitCounts["harbor"])
			}
			if row.FolderHitTotal != 1 {
				t.Fatalf("expected folder hit total 1, got %d", row.FolderHitTotal)
			}
		}
		if row.SourceRelativePath == "harbor-searchable" || row.SourceRelativePath == "harbor-searchable/unrelated.txt" {
			t.Fatalf("did not expect folder fallback for folder containing content-searchable files: %s", row.SourceRelativePath)
		}
		if row.SourceRelativePath == "harbor-folder/unrelated.bin" {
			if row.FilenameHitTotal != 0 || row.ContentHitTotal != 0 {
				t.Fatalf("expected no file-level hit for folder fallback child, got filename=%d content=%d", row.FilenameHitTotal, row.ContentHitTotal)
			}
		}
	}
	if !bodyMatchFound {
		t.Fatalf("expected manifest row for meeting-notes.txt")
	}
	if !folderMatchFound {
		t.Fatalf("expected manifest row for harbor-folder/unrelated.bin")
	}
}

func TestRunWithSummarySearchesZipMembersAndCopiesArchiveOnce(t *testing.T) {
	sourceDir := t.TempDir()
	outputDir := t.TempDir()
	zipPath := filepath.Join(sourceDir, "archives", "bundle.zip")
	mustWriteZip(t, zipPath, map[string]string{
		"docs/meeting-note.txt": "northwind appears here. northwind appears again.",
		"docs/northwind-photo.bin": "not content searchable",
	})

	summary, err := RunWithSummary(Config{
		SourceDir:   sourceDir,
		OutputDir:   outputDir,
		Terms:       []string{"northwind"},
		SearchScope: SearchScopeBoth,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if summary.MatchedFiles != 2 {
		t.Fatalf("expected 2 ZIP member matches, got %d", summary.MatchedFiles)
	}
	if summary.CopiedFiles != 1 {
		t.Fatalf("expected original ZIP copied once, got %d", summary.CopiedFiles)
	}
	if got := summary.FilenameHitsByKeyword["northwind"]; got != 1 {
		t.Fatalf("expected 1 internal filename hit, got %d", got)
	}
	if got := summary.ContentHitsByKeyword["northwind"]; got != 2 {
		t.Fatalf("expected 2 internal content hits, got %d", got)
	}

	var filenameRow, contentRow *MatchRow
	for i := range summary.ManifestRows {
		row := &summary.ManifestRows[i]
		if row.ArchivePath != "archives/bundle.zip" {
			t.Fatalf("expected archive path on ZIP member row, got %q", row.ArchivePath)
		}
		wantCopied := filepath.ToSlash(filepath.Join(summary.RunDir, "matched_files", "archives", "bundle.zip"))
		if row.CopiedFilePath != wantCopied {
			t.Fatalf("expected copied path %q, got %q", wantCopied, row.CopiedFilePath)
		}
		if row.ArchiveInternalPath == "docs/northwind-photo.bin" {
			filenameRow = row
		}
		if row.ArchiveInternalPath == "docs/meeting-note.txt" {
			contentRow = row
		}
	}
	if filenameRow == nil {
		t.Fatalf("expected internal filename match row")
	}
	if filenameRow.FilenameHitTotal != 1 || filenameRow.ContentHitTotal != 0 {
		t.Fatalf("expected filename-only ZIP member hit, got filename=%d content=%d", filenameRow.FilenameHitTotal, filenameRow.ContentHitTotal)
	}
	if contentRow == nil {
		t.Fatalf("expected internal content match row")
	}
	if contentRow.ContentHitTotal != 2 {
		t.Fatalf("expected content hit total 2, got %d", contentRow.ContentHitTotal)
	}
}

func TestRunWithSummaryGuardsZipMembers(t *testing.T) {
	sourceDir := t.TempDir()
	outputDir := t.TempDir()
	zipPath := filepath.Join(sourceDir, "archives", "guarded.zip")
	mustWriteZip(t, zipPath, map[string]string{
		"nested/northwind.zip":    "nested archive should not be opened",
		"../northwind-escape.txt": "unsafe path should be ignored",
	})

	summary, err := RunWithSummary(Config{
		SourceDir:   sourceDir,
		OutputDir:   outputDir,
		Terms:       []string{"northwind"},
		DryRun:      true,
		SearchScope: SearchScopeBoth,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if summary.MatchedFiles != 1 {
		t.Fatalf("expected only safe nested archive filename row, got %d", summary.MatchedFiles)
	}
	row := summary.ManifestRows[0]
	if row.ArchiveInternalPath != "nested/northwind.zip" {
		t.Fatalf("expected nested archive internal path, got %q", row.ArchiveInternalPath)
	}
	if row.ArchiveStatus != "nested_archive_skipped" {
		t.Fatalf("expected nested archive status, got %q", row.ArchiveStatus)
	}
	if len(summary.Warnings) == 0 {
		t.Fatalf("expected unsafe path warning")
	}
}

func TestFindHitsCountsRepeatedOccurrences(t *testing.T) {
	matches, counts, total := findHits("northwind northwind harbor northwind", []string{"northwind", "harbor", "gift"})

	if total != 4 {
		t.Fatalf("expected total 4, got %d", total)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matched terms, got %d", len(matches))
	}
	if counts["northwind"] != 3 {
		t.Fatalf("expected northwind count 3, got %d", counts["northwind"])
	}
	if counts["harbor"] != 1 {
		t.Fatalf("expected harbor count 1, got %d", counts["harbor"])
	}
}

func TestRunWithSummaryPathScopeSkipsContentAndStopsAtMax(t *testing.T) {
	sourceDir := t.TempDir()
	outputDir := t.TempDir()

	mustWriteFile(t, filepath.Join(sourceDir, "alpha-content.txt"), "alpha alpha")
	mustWriteFile(t, filepath.Join(sourceDir, "beta", "alpha-name.bin"), "not searchable")
	mustWriteFile(t, filepath.Join(sourceDir, "gamma", "alpha-second.bin"), "not searchable")

	summary, err := RunWithSummary(Config{
		SourceDir:   sourceDir,
		OutputDir:   outputDir,
		Terms:       []string{"alpha"},
		DryRun:      true,
		SearchScope: SearchScopePaths,
		MaxMatches:  2,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if summary.MatchedFiles != 2 {
		t.Fatalf("expected 2 matches, got %d", summary.MatchedFiles)
	}
	if !summary.StoppedByMaxMatches {
		t.Fatalf("expected run to stop at max matches")
	}
	if got := summary.ContentHitsByKeyword["alpha"]; got != 0 {
		t.Fatalf("expected path scope to skip content hits, got %d", got)
	}
	if got := summary.FilenameHitsByKeyword["alpha"]; got != 2 {
		t.Fatalf("expected 2 filename hits, got %d", got)
	}
}

func TestRunWithSummarySkipsOversizedContentWhenLimitSet(t *testing.T) {
	sourceDir := t.TempDir()
	outputDir := t.TempDir()

	mustWriteFile(t, filepath.Join(sourceDir, "large.txt"), "alpha alpha")

	summary, err := RunWithSummary(Config{
		SourceDir:       sourceDir,
		OutputDir:       outputDir,
		Terms:           []string{"alpha"},
		DryRun:          true,
		SearchScope:     SearchScopeContent,
		MaxContentBytes: 1,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if summary.MatchedFiles != 0 {
		t.Fatalf("expected no content matches after size skip, got %d", summary.MatchedFiles)
	}
	if summary.ContentSizeSkippedFiles != 1 {
		t.Fatalf("expected 1 size-skipped file, got %d", summary.ContentSizeSkippedFiles)
	}
}

func TestRunWithSummarySearchesPDFContentViaPDFToText(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub shell helper uses /bin/sh")
	}

	sourceDir := t.TempDir()
	outputDir := t.TempDir()
	stubDir := t.TempDir()
	installStubCommand(t, stubDir, "pdftotext", "#!/bin/sh\nprintf 'Northwind appears in PDF body\\n'")
	t.Setenv("PATH", stubDir)

	mustWriteFile(t, filepath.Join(sourceDir, "docs", "notes.pdf"), "%PDF-1.4 placeholder")

	summary, err := RunWithSummary(Config{
		SourceDir:   sourceDir,
		OutputDir:   outputDir,
		Terms:       []string{"northwind"},
		DryRun:      true,
		SearchScope: SearchScopeContent,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if summary.MatchedFiles != 1 {
		t.Fatalf("expected 1 PDF content match, got %d", summary.MatchedFiles)
	}
	if got := summary.ContentHitsByKeyword["northwind"]; got != 1 {
		t.Fatalf("expected 1 PDF content hit, got %d", got)
	}
	if summary.InventoryRows[0].SearchMethod != string(MethodPDFToText) {
		t.Fatalf("expected PDF search method %q, got %q", MethodPDFToText, summary.InventoryRows[0].SearchMethod)
	}
}

func TestRunWithSummaryWarnsWhenPDFExtractionFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub shell helper uses /bin/sh")
	}

	sourceDir := t.TempDir()
	outputDir := t.TempDir()
	stubDir := t.TempDir()
	installStubCommand(t, stubDir, "pdftotext", "#!/bin/sh\necho 'pdftotext exploded' >&2\nexit 1\n")
	t.Setenv("PATH", stubDir)

	mustWriteFile(t, filepath.Join(sourceDir, "docs", "notes.pdf"), "%PDF-1.4 placeholder")

	summary, err := RunWithSummary(Config{
		SourceDir:   sourceDir,
		OutputDir:   outputDir,
		Terms:       []string{"northwind"},
		DryRun:      true,
		SearchScope: SearchScopeContent,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if summary.MatchedFiles != 0 {
		t.Fatalf("expected 0 matches after failed PDF extraction, got %d", summary.MatchedFiles)
	}
	if len(summary.Warnings) == 0 {
		t.Fatalf("expected PDF extraction warning")
	}
	if got := summary.Warnings[0]; got != "docs/notes.pdf: pdftotext exploded" {
		t.Fatalf("expected stderr-backed PDF warning, got %q", got)
	}
}

func TestRunWithSummarySearchesLegacyOfficeContentViaSoffice(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub shell helper uses /bin/sh")
	}

	sourceDir := t.TempDir()
	outputDir := t.TempDir()
	stubDir := t.TempDir()
	installStubCommand(t, stubDir, "soffice", "#!/bin/sh\noutdir=''\ninput=''\nwhile [ \"$#\" -gt 0 ]; do\n  if [ \"$1\" = \"--outdir\" ]; then\n    outdir=\"$2\"\n    shift 2\n    continue\n  fi\n  input=\"$1\"\n  shift\ndone\nbase=\"${input##*/}\"\nname=\"${base%.*}\"\nprintf 'AgencyX appears in legacy office body\\n' > \"$outdir/$name.txt\"\n")
	t.Setenv("PATH", stubDir)

	mustWriteFile(t, filepath.Join(sourceDir, "docs", "legacy.doc"), "placeholder")

	summary, err := RunWithSummary(Config{
		SourceDir:   sourceDir,
		OutputDir:   outputDir,
		Terms:       []string{"agencyx"},
		DryRun:      true,
		SearchScope: SearchScopeContent,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if summary.MatchedFiles != 1 {
		t.Fatalf("expected 1 soffice content match, got %d", summary.MatchedFiles)
	}
	if got := summary.ContentHitsByKeyword["agencyx"]; got != 1 {
		t.Fatalf("expected 1 soffice content hit, got %d", got)
	}
	if summary.InventoryRows[0].SearchMethod != string(MethodSofficeText) {
		t.Fatalf("expected soffice search method %q, got %q", MethodSofficeText, summary.InventoryRows[0].SearchMethod)
	}
}

func TestRunWithSummaryWarnsWhenSofficeExtractionFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub shell helper uses /bin/sh")
	}

	sourceDir := t.TempDir()
	outputDir := t.TempDir()
	stubDir := t.TempDir()
	installStubCommand(t, stubDir, "soffice", "#!/bin/sh\necho 'soffice exploded' >&2\nexit 1\n")
	t.Setenv("PATH", stubDir)

	mustWriteFile(t, filepath.Join(sourceDir, "docs", "legacy.doc"), "placeholder")

	summary, err := RunWithSummary(Config{
		SourceDir:   sourceDir,
		OutputDir:   outputDir,
		Terms:       []string{"agencyx"},
		DryRun:      true,
		SearchScope: SearchScopeContent,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if summary.MatchedFiles != 0 {
		t.Fatalf("expected 0 matches after failed soffice extraction, got %d", summary.MatchedFiles)
	}
	if len(summary.Warnings) == 0 {
		t.Fatalf("expected soffice extraction warning")
	}
}

func TestRunWithSummaryWarnsWhenSofficeProducesNoTextFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub shell helper uses /bin/sh")
	}

	sourceDir := t.TempDir()
	outputDir := t.TempDir()
	stubDir := t.TempDir()
	installStubCommand(t, stubDir, "soffice", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir)

	mustWriteFile(t, filepath.Join(sourceDir, "docs", "legacy.doc"), "placeholder")

	summary, err := RunWithSummary(Config{
		SourceDir:   sourceDir,
		OutputDir:   outputDir,
		Terms:       []string{"agencyx"},
		DryRun:      true,
		SearchScope: SearchScopeContent,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if summary.MatchedFiles != 0 {
		t.Fatalf("expected 0 matches after missing soffice text output, got %d", summary.MatchedFiles)
	}
	if len(summary.Warnings) == 0 {
		t.Fatalf("expected soffice missing-output warning")
	}
	if got := summary.Warnings[0]; got != "docs/legacy.doc: soffice conversion completed but did not produce a text file" {
		t.Fatalf("unexpected soffice missing-output warning %q", got)
	}
}

func TestRunWithSummaryDoesNotUseFolderYearAheadOfFileDate(t *testing.T) {
	sourceDir := t.TempDir()
	outputDir := t.TempDir()

	mustWriteFile(t, filepath.Join(sourceDir, "archive", "2020", "large.txt"), "alpha alpha")
	start := mustParseDay(t, "2022-01-01")
	end := mustParseDay(t, "2025-01-31")

	summary, err := RunWithSummary(Config{
		SourceDir:       sourceDir,
		OutputDir:       outputDir,
		Terms:           []string{"alpha"},
		DryRun:          true,
		SearchScope:     SearchScopeBoth,
		StartDate:       start,
		EndDate:         end,
		MaxContentBytes: 1,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if summary.MatchedFiles != 0 {
		t.Fatalf("expected 0 matches because content size limits still apply, got %d", summary.MatchedFiles)
	}
	if summary.ExcludedPreRangeFiles != 0 {
		t.Fatalf("expected 0 pre-range exclusions, got %d", summary.ExcludedPreRangeFiles)
	}
	if summary.ContentSizeSkippedFiles != 1 {
		t.Fatalf("expected content evaluation to use file-level metadata first, got %d size skips", summary.ContentSizeSkippedFiles)
	}
}

func TestRunWithSnapshotUsesPrescanInventory(t *testing.T) {
	sourceDir := t.TempDir()
	outputDir := t.TempDir()

	mustWriteFile(t, filepath.Join(sourceDir, "docs", "alpha.txt"), "alpha")

	snapshot, err := BuildInventorySnapshot(sourceDir)
	if err != nil {
		t.Fatal(err)
	}

	summary, err := RunWithSnapshot(Config{
		SourceDir:   sourceDir,
		OutputDir:   outputDir,
		Terms:       []string{"alpha"},
		DryRun:      true,
		SearchScope: SearchScopeBoth,
	}, nil, snapshot)
	if err != nil {
		t.Fatal(err)
	}

	if !summary.UsedInventorySnapshot {
		t.Fatalf("expected run to use prescan inventory snapshot")
	}
	if summary.MatchedFiles != 1 {
		t.Fatalf("expected 1 match, got %d", summary.MatchedFiles)
	}
}

func TestFolderFallbackSkippedWhenDescendantFilenameHitExists(t *testing.T) {
	sourceDir := t.TempDir()
	outputDir := t.TempDir()

	mustWriteFile(t, filepath.Join(sourceDir, "alpha-folder", "alpha-photo.bin"), "not searchable")

	summary, err := RunWithSummary(Config{
		SourceDir:   sourceDir,
		OutputDir:   outputDir,
		Terms:       []string{"alpha"},
		DryRun:      true,
		SearchScope: SearchScopePaths,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if got := summary.FilenameHitsByKeyword["alpha"]; got != 1 {
		t.Fatalf("expected filename hit, got %d", got)
	}
	if got := summary.FolderHitsByKeyword["alpha"]; got != 0 {
		t.Fatalf("expected no folder fallback when descendant filename hit exists, got %d", got)
	}
	for _, row := range summary.ManifestRows {
		if row.Extension == "[folder]" {
			t.Fatalf("did not expect folder fallback row: %s", row.SourceRelativePath)
		}
	}
}

func TestFormatHitSummaryCombinesFilenameAndContentCounts(t *testing.T) {
	row := MatchRow{
		FilenameHits:      []string{"Northwind"},
		FilenameHitCounts: map[string]int{"Northwind": 1},
		FolderHits:        []string{"AgencyX"},
		FolderHitCounts:   map[string]int{"AgencyX": 1},
		ContentHits:       []string{"Northwind", "Budget"},
		ContentHitCounts:  map[string]int{"Northwind": 2, "Budget": 24},
	}

	got := formatHitSummary(row)
	want := "filename:Northwind=1 ; folder:AgencyX=1 ; content:Northwind=2 ; content:Budget=24"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestClassifyDocumentDatePreRangeExcluded(t *testing.T) {
	start := mustParseDay(t, "2022-01-01")
	end := mustParseDay(t, "2025-01-31")
	selected := mustParseTime(t, "2021-12-31T23:00:00Z")

	meta := dateMetadata{
		Selected:       selected,
		SelectedSource: "filesystem_modified",
	}
	classifyDocumentDate(&meta, start, end)

	if meta.Status != DateStatusExcludedPreRange {
		t.Fatalf("expected excluded_pre_range, got %s", meta.Status)
	}
}

func TestClassifyDocumentDatePostRangeUnknown(t *testing.T) {
	start := mustParseDay(t, "2022-01-01")
	end := mustParseDay(t, "2025-01-31")
	selected := mustParseTime(t, "2025-02-01T00:00:00Z")

	meta := dateMetadata{
		Selected:       selected,
		SelectedSource: "filesystem_created",
	}
	classifyDocumentDate(&meta, start, end)

	if meta.Status != DateStatusUnknown {
		t.Fatalf("expected unknown, got %s", meta.Status)
	}
}

func TestClassifyDocumentDateMissingUnknown(t *testing.T) {
	start := mustParseDay(t, "2022-01-01")
	end := mustParseDay(t, "2025-01-31")

	meta := dateMetadata{}
	classifyDocumentDate(&meta, start, end)

	if meta.Status != DateStatusUnknown {
		t.Fatalf("expected unknown, got %s", meta.Status)
	}
}

func TestExtractFolderYearFindsNearestDirectoryYear(t *testing.T) {
	year, ok := extractFolderYear("SER-010E/Records Requests/2017/Example/Public 15.pdf")
	if !ok {
		t.Fatalf("expected folder year")
	}
	if year != 2017 {
		t.Fatalf("expected 2017, got %d", year)
	}
}

func TestApplyFolderYearExcludesPreRange(t *testing.T) {
	start := mustParseDay(t, "2022-01-01")
	end := mustParseDay(t, "2025-01-31")
	meta := dateMetadata{}

	applyFolderYear(&meta, 2020, start, end)

	if meta.Status != DateStatusExcludedPreRange {
		t.Fatalf("expected excluded_pre_range, got %s", meta.Status)
	}
}

func TestApplyFolderYearEndBoundaryIncluded(t *testing.T) {
	start := mustParseDay(t, "2022-01-01")
	end := mustParseDay(t, "2025-01-31")
	meta := dateMetadata{}

	applyFolderYear(&meta, 2025, start, end)

	if meta.Status != DateStatusInRange {
		t.Fatalf("expected in_range, got %s", meta.Status)
	}
}

func TestApplyFolderYearFallbackOnlyWhenNoFileDateExists(t *testing.T) {
	fileDate := mustParseTime(t, "2024-04-01T00:00:00Z")
	meta := dateMetadata{
		Selected:       fileDate,
		SelectedSource: "filesystem_modified",
	}

	applyFolderYearFallback(&meta, "archive/2020/large.txt")

	if meta.SelectedSource != "filesystem_modified" {
		t.Fatalf("expected existing file-level source to remain selected, got %q", meta.SelectedSource)
	}
	if meta.Selected == nil || !meta.Selected.Equal(*fileDate) {
		t.Fatalf("expected existing file-level date to remain selected")
	}
}

func TestApplyFolderYearFallbackUsesPathYearWhenNoFileDateExists(t *testing.T) {
	meta := dateMetadata{}

	applyFolderYearFallback(&meta, "archive/2020/large.txt")

	if meta.SelectedSource != "folder_year" {
		t.Fatalf("expected folder_year fallback, got %q", meta.SelectedSource)
	}
	if meta.Selected == nil || meta.Selected.Year() != 2020 {
		t.Fatalf("expected folder year 2020 to be selected")
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func installStubCommand(t *testing.T, dir, name, script string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
}

func mustWriteZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func headerIndexMap(headers []string) map[string]int {
	indexes := make(map[string]int, len(headers))
	for i, header := range headers {
		indexes[header] = i
	}
	return indexes
}

func mustParseDay(t *testing.T, value string) *time.Time {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		t.Fatal(err)
	}
	return &parsed
}

func mustParseTime(t *testing.T, value string) *time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return &parsed
}
