package scanner

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xuri/excelize/v2"
)

// DiscoverFiles walks the source directory and returns files grouped by type.
// Only returns file types that are in enabledTypes.
func DiscoverFiles(sourceDir string, enabledTypes map[FileType]bool) (map[FileType][]string, error) {
	result := make(map[FileType][]string)

	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable
		}
		if info.IsDir() {
			if path != sourceDir && ShouldSkipWalkDir(path) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(info.Name(), "._") {
			return nil // skip macOS AppleDouble sidecar files
		}

		for _, ft := range AllFileTypes() {
			if !enabledTypes[ft] {
				continue
			}
			if ft.MatchesExtension(info.Name()) {
				result[ft] = append(result[ft], path)
				break
			}
		}
		return nil
	})

	for _, files := range result {
		sort.Slice(files, func(i, j int) bool {
			return strings.ToLower(files[i]) < strings.ToLower(files[j])
		})
	}

	return result, err
}

// CountFiles returns a count per file type from the discovered files map.
func CountFiles(files map[FileType][]string) map[FileType]int {
	counts := make(map[FileType]int)
	for ft, list := range files {
		counts[ft] = len(list)
	}
	return counts
}

// TotalFileCount returns the total number of files across all types.
func TotalFileCount(files map[FileType][]string) int {
	total := 0
	for _, list := range files {
		total += len(list)
	}
	return total
}

type runArtifacts struct {
	Timestamp            string
	RunDir               string
	ReportPath           string
	ManifestPath         string
	ReviewManifestPath   string
	ManifestWorkbookPath string
	ReviewWorkbookPath   string
	ConfigPath           string
}

type sourceFile struct {
	path     string
	relPath  string
	fileType FileType
}

type messageInfo struct {
	path          string
	relativePath  string
	relativeDir   string
	stem          string
	searchable    bool
	unknownDate   bool
	unknownReason string
}

func Run(cfg Config, events chan<- Event) error {
	_, err := RunWithSummary(cfg, events)
	return err
}

func RunWithSummary(cfg Config, events chan<- Event) (*RunSummary, error) {
	if events != nil {
		defer close(events)
	}
	PrepareMacOutputTree()

	if err := ValidateRoots(cfg.SourceDir, cfg.OutputDir); err != nil {
		return nil, err
	}
	info, err := os.Stat(cfg.SourceDir)
	if err != nil {
		return nil, fmt.Errorf("failed to access source directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("source path must be a directory")
	}

	artifacts, err := prepareRunArtifacts(cfg.OutputDir)
	if err != nil {
		return nil, err
	}
	defer CleanupAppleDoubleArtifacts(artifacts.RunDir)

	logger, err := NewLogger(artifacts.RunDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}
	defer logger.Close()

	mode := "scan"
	if cfg.DryRun {
		mode = "estimate"
	}

	summary := &RunSummary{
		RunTimestamp:         artifacts.Timestamp,
		RunDir:               filepath.Base(artifacts.RunDir),
		ReportPath:           RelPath(cfg.OutputDir, artifacts.ReportPath),
		ManifestPath:         RelPath(cfg.OutputDir, artifacts.ManifestPath),
		ReviewManifestPath:   RelPath(cfg.OutputDir, artifacts.ReviewManifestPath),
		ManifestWorkbookPath: RelPath(cfg.OutputDir, artifacts.ManifestWorkbookPath),
		ReviewWorkbookPath:   RelPath(cfg.OutputDir, artifacts.ReviewWorkbookPath),
		ConfigPath:           RelPath(cfg.OutputDir, artifacts.ConfigPath),
		LogPath:              RelPath(cfg.OutputDir, filepath.Join(artifacts.RunDir, "script_log.txt")),
		Mode:                 mode,
		SourceRootLabel:      filepath.Base(cfg.SourceDir),
		OutputRootLabel:      filepath.Base(cfg.OutputDir),
		Terms:                append([]string(nil), cfg.Terms...),
		RejectedKeywords:     append([]RejectedKeyword(nil), cfg.RejectedKeywords...),
		HasDateFilter:        cfg.StartDate != nil || cfg.EndDate != nil,
		FileCounts:           make(map[FileType]int),
		KeywordHits:          make(map[string]int),
		UnknownDateHits:      make(map[string]int),
		HitsBySource:         make(map[string]int),
		HitsByType:           make(map[string]int),
		FilesByType:          make(map[FileType][]string),
		ManifestRows:         make([]ManifestRow, 0),
		FlaggedDirs:          make([]string, 0),
		Warnings:             make([]string, 0),
		Errors:               make([]string, 0),
		SkippedFormats:       make([]string, 0),
		HasReadPST:           HasReadPST(),
		HasHighFidelityMSG:   HasHighFidelityMSG(),
	}
	if cfg.StartDate != nil {
		summary.StartDate = cfg.StartDate.Format("2006-01-02")
	}
	if cfg.EndDate != nil {
		summary.EndDate = cfg.EndDate.Format("2006-01-02")
	}

	logger.Log(fmt.Sprintf("=== Keyword Hunter %s started at %s ===", strings.Title(mode), Timestamp()))
	logger.Log(fmt.Sprintf("Source root: %s", summary.SourceRootLabel))
	logger.Log(fmt.Sprintf("Output root: %s", summary.OutputRootLabel))
	logger.Log(fmt.Sprintf("Run directory: %s", summary.RunDir))
	if len(cfg.Terms) > 0 {
		logger.Log(fmt.Sprintf("Terms: %s", strings.Join(cfg.Terms, ", ")))
	}
	if summary.HasDateFilter {
		logger.Log(fmt.Sprintf("Date filter: start=%s end=%s", emptyIfBlank(summary.StartDate), emptyIfBlank(summary.EndDate)))
	}
	logger.Log("")

	flagged, _ := ScanFlaggedFolders(cfg.SourceDir)
	for _, dir := range flagged {
		summary.FlaggedDirs = append(summary.FlaggedDirs, RelPath(cfg.SourceDir, dir))
	}
	if len(summary.FlaggedDirs) > 0 {
		if err := WriteFlaggedFolders(artifacts.RunDir, summary.FlaggedDirs); err == nil {
			logger.Log(fmt.Sprintf("Flagged %d folder(s) — see flagged_folders.txt", len(summary.FlaggedDirs)))
		}
		for _, f := range summary.FlaggedDirs {
			logger.Log(fmt.Sprintf("  FLAGGED: %s", f))
		}
		logger.Log("")
	}

	discovered, err := DiscoverFiles(cfg.SourceDir, cfg.EnabledTypes)
	if err != nil {
		return nil, fmt.Errorf("error discovering files: %w", err)
	}
	for ft, files := range discovered {
		summary.FilesByType[ft] = make([]string, 0, len(files))
		for _, file := range files {
			summary.FilesByType[ft] = append(summary.FilesByType[ft], RelPath(cfg.SourceDir, file))
		}
	}
	summary.FileCounts = CountFiles(discovered)
	summary.TotalFiles = TotalFileCount(discovered)

	effectiveEnabled := cloneEnabledTypes(cfg.EnabledTypes)
	handleMissingDependencies(cfg.SourceDir, discovered, effectiveEnabled, summary, logger)

	allFiles := buildProcessingList(cfg.SourceDir, discovered, effectiveEnabled)
	if events != nil {
		events <- Event{
			Type:       EventDiscovery,
			Counts:     summary.FileCounts,
			Flagged:    summary.FlaggedDirs,
			TotalFiles: len(allFiles),
			Message:    fmt.Sprintf("Discovered %d source file(s) to process", len(allFiles)),
		}
	}

	for _, ft := range AllFileTypes() {
		if cfg.EnabledTypes[ft] {
			logger.Log(fmt.Sprintf("Found %d %s file(s)", summary.FileCounts[ft], ft))
		}
	}
	logger.Log("")

	for idx, sf := range allFiles {
		if err := processSourceFile(cfg, artifacts, summary, logger, events, sf, idx+1, len(allFiles)); err != nil {
			summary.Errors = append(summary.Errors, err.Error())
			if events != nil {
				events <- Event{Type: EventError, Message: err.Error()}
			}
		}
	}

	logger.Log(fmt.Sprintf("=== %s complete. Processed %d source file(s). ===", strings.Title(mode), summary.FilesScanned))
	logger.Log("Run artifacts written.")

	if err := writeRunManifest(summary, cfg.OutputDir, artifacts.ManifestPath); err != nil {
		return nil, err
	}
	if err := writeReviewManifest(summary, cfg.OutputDir, artifacts.ReviewManifestPath); err != nil {
		return nil, err
	}
	if err := writeRunManifestWorkbook(summary, cfg.OutputDir, artifacts.ManifestWorkbookPath); err != nil {
		return nil, err
	}
	if err := writeReviewManifestWorkbook(summary, cfg.OutputDir, artifacts.ReviewWorkbookPath); err != nil {
		return nil, err
	}
	if err := writeRunConfig(summary, artifacts.ConfigPath); err != nil {
		return nil, err
	}
	if err := writeRunReport(summary, cfg.OutputDir, artifacts, artifacts.ReportPath); err != nil {
		return nil, err
	}

	if events != nil {
		events <- Event{
			Type:       EventComplete,
			TotalFiles: summary.FilesScanned,
			OutputDir:  summary.RunDir,
			Message:    fmt.Sprintf("%s complete. Report: %s", strings.Title(mode), summary.ReportPath),
		}
	}
	return summary, nil
}

func prepareRunArtifacts(outputRoot string) (*runArtifacts, error) {
	timestamp := TimestampForFilename()
	runDir := filepath.Join(outputRoot, timestamp)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create run directory: %w", err)
	}
	return &runArtifacts{
		Timestamp:            timestamp,
		RunDir:               runDir,
		ReportPath:           filepath.Join(runDir, fmt.Sprintf("run_report_%s.md", timestamp)),
		ManifestPath:         filepath.Join(runDir, fmt.Sprintf("match_manifest_%s.csv", timestamp)),
		ReviewManifestPath:   filepath.Join(runDir, fmt.Sprintf("review_manifest_%s.csv", timestamp)),
		ManifestWorkbookPath: filepath.Join(runDir, fmt.Sprintf("match_manifest_%s.xlsx", timestamp)),
		ReviewWorkbookPath:   filepath.Join(runDir, fmt.Sprintf("review_manifest_%s.xlsx", timestamp)),
		ConfigPath:           filepath.Join(runDir, fmt.Sprintf("run_config_%s.json", timestamp)),
	}, nil
}

func handleMissingDependencies(sourceDir string, discovered map[FileType][]string, enabled map[FileType]bool, summary *RunSummary, logger *Logger) {
	if enabled[TypePST] && len(discovered[TypePST]) > 0 && !summary.HasReadPST {
		disableType(sourceDir, TypePST, discovered, enabled, summary, logger, "Required dependency missing: readpst is not installed.")
	}
	if enabled[TypeOST] && len(discovered[TypeOST]) > 0 && !summary.HasReadPST {
		disableType(sourceDir, TypeOST, discovered, enabled, summary, logger, "Required dependency missing: readpst is not installed.")
	}
	if enabled[TypeMSG] && len(discovered[TypeMSG]) > 0 && !summary.HasHighFidelityMSG {
		disableType(sourceDir, TypeMSG, discovered, enabled, summary, logger, "Required dependency missing: high-fidelity MSG support is unavailable because extract-msg is not installed.")
	}
}

func disableType(sourceDir string, ft FileType, discovered map[FileType][]string, enabled map[FileType]bool, summary *RunSummary, logger *Logger, reason string) {
	if !enabled[ft] {
		return
	}
	enabled[ft] = false
	formatLine := fmt.Sprintf("%s disabled: %s", ft, reason)
	summary.SkippedFormats = append(summary.SkippedFormats, formatLine)
	summary.Warnings = append(summary.Warnings, formatLine)
	logger.Log("WARNING: " + formatLine)
	for _, file := range discovered[ft] {
		summary.SkippedTotal++
		summary.ManifestRows = append(summary.ManifestRows, ManifestRow{
			RunTimestamp:        summary.RunTimestamp,
			Status:              "skipped_dependency",
			SourceType:          ft.String(),
			SourceContainerPath: RelPath(sourceDir, file),
			Keyword:             "",
			KeywordDir:          "",
			Note:                reason,
		})
	}
}

func buildProcessingList(sourceDir string, discovered map[FileType][]string, enabled map[FileType]bool) []sourceFile {
	var allFiles []sourceFile
	for _, ft := range []FileType{TypePST, TypeOST, TypeMBOX, TypeEML, TypeMSG} {
		if !enabled[ft] {
			continue
		}
		for _, path := range discovered[ft] {
			allFiles = append(allFiles, sourceFile{
				path:     path,
				relPath:  RelPath(sourceDir, path),
				fileType: ft,
			})
		}
	}
	return allFiles
}

func processSourceFile(cfg Config, artifacts *runArtifacts, summary *RunSummary, logger *Logger, events chan<- Event, sf sourceFile, fileNum, totalFiles int) error {
	summary.FilesScanned++
	dirName := MakeSourceDirName(fileNum, filepath.Base(sf.path))
	sourceOutDir := filepath.Join(artifacts.RunDir, dirName)

	logger.Log(fmt.Sprintf("--- [%04d] %s: %s ---", fileNum, sf.fileType, sf.relPath))
	logger.Log(fmt.Sprintf("    Output: %s/", filepath.ToSlash(filepath.Join(summary.RunDir, dirName))))

	if events != nil {
		events <- Event{
			Type:       EventFileStart,
			SourceFile: sf.relPath,
			SourceType: sf.fileType,
			OutputDir:  filepath.ToSlash(filepath.Join(summary.RunDir, dirName)),
			FileNum:    fileNum,
			TotalFiles: totalFiles,
			Message:    fmt.Sprintf("[%04d] %s: %s", fileNum, sf.fileType, sf.relPath),
		}
	}

	switch sf.fileType {
	case TypePST, TypeOST:
		tmpDir, err := os.MkdirTemp("", "kh-extract-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpDir)
		if events != nil {
			events <- Event{
				Type:       EventExtracting,
				SourceFile: sf.relPath,
				FileNum:    fileNum,
				TotalFiles: totalFiles,
				Message:    "Extracting " + sf.relPath,
			}
		}
		if err := ExtractPST(sf.path, tmpDir, logger); err != nil {
			note := fmt.Sprintf("Source skipped: %s", err.Error())
			recordSourceSkip(summary, artifacts, cfg.OutputDir, sf, dirName, note)
			return fmt.Errorf("%s", note)
		}
		if err := processEMLDir(cfg, artifacts, summary, logger, events, sf, dirName, sourceOutDir, tmpDir, fileNum, totalFiles); err != nil {
			return err
		}
	case TypeEML:
		tmpDir, err := os.MkdirTemp("", "kh-eml-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpDir)
		data, err := os.ReadFile(sf.path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(tmpDir, filepath.Base(sf.path))
		if err := os.WriteFile(destPath, data, 0644); err != nil {
			return err
		}
		if err := processEMLDir(cfg, artifacts, summary, logger, events, sf, dirName, sourceOutDir, tmpDir, fileNum, totalFiles); err != nil {
			return err
		}
	case TypeMSG:
		tmpDir, err := os.MkdirTemp("", "kh-msg-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpDir)
		if _, err := ConvertMSGToEML(sf.path, tmpDir); err != nil {
			note := fmt.Sprintf("MSG skipped: %s", err.Error())
			recordSourceSkip(summary, artifacts, cfg.OutputDir, sf, dirName, note)
			return fmt.Errorf("%s", note)
		}
		if err := processEMLDir(cfg, artifacts, summary, logger, events, sf, dirName, sourceOutDir, tmpDir, fileNum, totalFiles); err != nil {
			return err
		}
	case TypeMBOX:
		tmpDir, err := os.MkdirTemp("", "kh-mbox-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpDir)
		if events != nil {
			events <- Event{
				Type:       EventExtracting,
				SourceFile: sf.relPath,
				FileNum:    fileNum,
				TotalFiles: totalFiles,
				Message:    "Splitting MBOX " + sf.relPath,
			}
		}
		if _, err := SplitMBOX(sf.path, tmpDir); err != nil {
			note := fmt.Sprintf("MBOX skipped: %s", err.Error())
			recordSourceSkip(summary, artifacts, cfg.OutputDir, sf, dirName, note)
			return fmt.Errorf("%s", note)
		}
		if err := processEMLDir(cfg, artifacts, summary, logger, events, sf, dirName, sourceOutDir, tmpDir, fileNum, totalFiles); err != nil {
			return err
		}
	}

	if events != nil {
		events <- Event{
			Type:       EventFileDone,
			SourceFile: sf.relPath,
			FileNum:    fileNum,
			TotalFiles: totalFiles,
			Message:    fmt.Sprintf("Done: %s", sf.relPath),
		}
	}
	logger.Log("")
	return nil
}

func processEMLDir(cfg Config, artifacts *runArtifacts, summary *RunSummary, logger *Logger, events chan<- Event, sf sourceFile, dirName, sourceOutDir, emlDir string, fileNum, totalFiles int) error {
	emlFiles, err := FindEMLFiles(emlDir)
	if err != nil {
		return fmt.Errorf("error finding EML files: %w", err)
	}
	if len(emlFiles) == 0 {
		logger.Log("  No EML files found in extracted source")
		return nil
	}

	messageInfos := make([]messageInfo, 0, len(emlFiles))
	for _, emlPath := range emlFiles {
		relPath := RelPath(emlDir, emlPath)
		relDir := SafeJoinRelative(filepath.Dir(relPath))
		if relDir == "." {
			relDir = ""
		}
		info := messageInfo{
			path:         emlPath,
			relativePath: filepath.ToSlash(relPath),
			relativeDir:  relDir,
			stem:         strings.TrimSuffix(filepath.Base(emlPath), filepath.Ext(emlPath)),
			searchable:   true,
		}
		if cfg.StartDate != nil || cfg.EndDate != nil {
			msgDate, err := ParseMessageDate(emlPath)
			switch {
			case err != nil || msgDate == nil:
				info.unknownDate = true
				info.unknownReason = "message date could not be parsed"
			case !DateInRange(*msgDate, cfg.StartDate, cfg.EndDate):
				info.searchable = false
			}
		}
		messageInfos = append(messageInfos, info)
	}

	for _, term := range cfg.Terms {
		logger.Log("Searching for term: " + term)
		if events != nil {
			events <- Event{
				Type:       EventSearching,
				Term:       term,
				SourceFile: sf.relPath,
				FileNum:    fileNum,
				TotalFiles: totalFiles,
				Message:    "Searching for term: " + term,
			}
		}

		for _, info := range messageInfos {
			if !info.searchable && !info.unknownDate {
				continue
			}
			matched, err := ContainsKeyword(info.path, term)
			if err != nil {
				warn := fmt.Sprintf("Warning: error reading %s: %s", info.relativePath, err)
				logger.Log("  " + warn)
				summary.Warnings = append(summary.Warnings, warn)
				continue
			}
			if !matched {
				continue
			}
			hitLocations, locationErr := FindKeywordLocations(info.path, term)
			if locationErr != nil {
				warn := fmt.Sprintf("Warning: error locating hits in %s: %s", info.relativePath, locationErr)
				logger.Log("  " + warn)
				summary.Warnings = append(summary.Warnings, warn)
			}

			keywordDir := TermToDirname(term)
			targetBase := filepath.Join(sourceOutDir, keywordDir)
			status := "exported"
			note := ""
			if info.relativeDir != "" {
				targetBase = filepath.Join(targetBase, info.relativeDir)
			}
			if info.unknownDate {
				targetBase = filepath.Join(artifacts.RunDir, "unknown_date", dirName, keywordDir)
				if info.relativeDir != "" {
					targetBase = filepath.Join(targetBase, info.relativeDir)
				}
				status = "unknown_date"
				note = info.unknownReason
				summary.UnknownDateHits[term]++
				summary.UnknownDateTotal++
			}

			headerPath := filepath.Join(targetBase, info.stem+"_header.txt")
			emlOutPath := filepath.Join(targetBase, info.stem+".eml")
			if cfg.DryRun {
				if status == "unknown_date" {
					status = "estimated_unknown_date"
				} else {
					status = "estimated_export"
				}
			} else {
				if err := os.MkdirAll(targetBase, 0755); err != nil {
					return err
				}
				header, headerErr := ExtractHeader(info.path)
				if headerErr == nil {
					if err := os.WriteFile(headerPath, []byte(header), 0644); err != nil {
						return err
					}
				}
				data, readErr := os.ReadFile(info.path)
				if readErr != nil {
					return readErr
				}
				if err := os.WriteFile(emlOutPath, data, 0644); err != nil {
					return err
				}
			}

			summary.KeywordHits[term]++
			summary.HitsBySource[sf.relPath]++
			summary.HitsByType[sf.fileType.String()]++

			matchMsg := "Found a match for term: " + term + " in " + info.relativePath
			logger.Log(matchMsg)
			row := ManifestRow{
				RunTimestamp:        summary.RunTimestamp,
				SourceType:          sf.fileType.String(),
				SourceContainerPath: sf.relPath,
				SourceContainerDir:  dirName,
				MessageBaseName:     filepath.Base(info.relativePath),
				MessageDirPath:      messageDirectoryPath(info.relativePath),
				Keyword:             term,
				KeywordDir:          keywordDir,
				HitLocations:        strings.Join(hitLocations, "|"),
				MessageRelativePath: info.relativePath,
				OutputEMLPath:       RelPath(cfg.OutputDir, emlOutPath),
				OutputHeaderPath:    RelPath(cfg.OutputDir, headerPath),
				SizeBytes:           messageSize(info.path),
				Status:              status,
				Note:                note,
			}
			summary.ManifestRows = append(summary.ManifestRows, row)

			if events != nil {
				eventType := EventMatch
				if info.unknownDate {
					eventType = EventUnknownDate
				}
				events <- Event{
					Type:       eventType,
					Term:       term,
					SourceFile: sf.relPath,
					OutputDir:  row.OutputEMLPath,
					FileNum:    fileNum,
					TotalFiles: totalFiles,
					Message:    matchMsg,
					Note:       note,
				}
			}
		}

		logger.Log("Finished searching for term: " + term)
		if events != nil {
			events <- Event{
				Type:       EventSearchDone,
				Term:       term,
				SourceFile: sf.relPath,
				FileNum:    fileNum,
				TotalFiles: totalFiles,
				Message:    "Finished searching for term: " + term,
			}
		}
	}
	return nil
}

func recordSourceSkip(summary *RunSummary, artifacts *runArtifacts, outputRoot string, sf sourceFile, dirName, note string) {
	summary.SkippedTotal++
	summary.Warnings = append(summary.Warnings, note)
	summary.ManifestRows = append(summary.ManifestRows, ManifestRow{
		RunTimestamp:        summary.RunTimestamp,
		SourceType:          sf.fileType.String(),
		SourceContainerPath: sf.relPath,
		SourceContainerDir:  dirName,
		MessageBaseName:     "",
		MessageDirPath:      "",
		Status:              "skipped_source",
		Note:                note,
		OutputEMLPath:       RelPath(outputRoot, filepath.Join(artifacts.RunDir, dirName)),
	})
}

func writeRunManifest(summary *RunSummary, outputRoot, manifestPath string) error {
	return writeCSV(manifestPath, technicalManifestHeaders(), technicalManifestRecords(summary.ManifestRows))
}

func writeReviewManifest(summary *RunSummary, outputRoot, manifestPath string) error {
	return writeCSV(manifestPath, reviewManifestHeaders(), reviewManifestRecords(summary.ManifestRows))
}

func writeRunManifestWorkbook(summary *RunSummary, outputRoot, path string) error {
	return writeWorkbook(path, "Technical Manifest", technicalManifestHeaders(), technicalManifestRecords(summary.ManifestRows), "TechnicalManifest")
}

func writeReviewManifestWorkbook(summary *RunSummary, outputRoot, path string) error {
	return writeWorkbook(path, "Reviewer Manifest", reviewManifestHeaders(), reviewManifestRecords(summary.ManifestRows), "ReviewerManifest")
}

func writeRunConfig(summary *RunSummary, configPath string) error {
	payload := map[string]any{
		"run_timestamp":          summary.RunTimestamp,
		"run_dir":                summary.RunDir,
		"mode":                   summary.Mode,
		"manifest_path":          summary.ManifestPath,
		"review_manifest_path":   summary.ReviewManifestPath,
		"manifest_workbook_path": summary.ManifestWorkbookPath,
		"review_workbook_path":   summary.ReviewWorkbookPath,
		"source_root_label":      summary.SourceRootLabel,
		"output_root_label":      summary.OutputRootLabel,
		"terms":                  summary.Terms,
		"rejected_keywords":      summary.RejectedKeywords,
		"start_date":             summary.StartDate,
		"end_date":               summary.EndDate,
		"has_date_filter":        summary.HasDateFilter,
		"has_readpst":            summary.HasReadPST,
		"has_high_fidelity_msg":  summary.HasHighFidelityMSG,
		"skipped_formats":        summary.SkippedFormats,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}

func writeRunReport(summary *RunSummary, outputRoot string, artifacts *runArtifacts, reportPath string) error {
	f, err := os.Create(reportPath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := func(s string) { fmt.Fprintln(f, s) }

	w("# Keyword Hunter Run Report")
	w("")
	w(fmt.Sprintf("Run timestamp: `%s`", summary.RunTimestamp))
	w(fmt.Sprintf("Mode: `%s`", summary.Mode))
	w(fmt.Sprintf("Source root: `%s`", summary.SourceRootLabel))
	w(fmt.Sprintf("Output root: `%s`", summary.OutputRootLabel))
	w(fmt.Sprintf("Run folder: `%s`", summary.RunDir))
	w(fmt.Sprintf("Technical manifest: `%s`", filepath.Base(summary.ManifestPath)))
	w(fmt.Sprintf("Reviewer manifest: `%s`", filepath.Base(summary.ReviewManifestPath)))
	w(fmt.Sprintf("Technical workbook: `%s`", filepath.Base(summary.ManifestWorkbookPath)))
	w(fmt.Sprintf("Reviewer workbook: `%s`", filepath.Base(summary.ReviewWorkbookPath)))
	w("")

	w("## Summary")
	w("")
	w(fmt.Sprintf("- Keywords: %d", len(summary.Terms)))
	w(fmt.Sprintf("- Total discovered source files: %d", summary.TotalFiles))
	w(fmt.Sprintf("- Source files processed this run: %d", summary.FilesScanned))
	w(fmt.Sprintf("- Total keyword-hit exports: %d", sumKeywordHits(summary.KeywordHits)))
	w(fmt.Sprintf("- Unknown-date hit count: %d", summary.UnknownDateTotal))
	w(fmt.Sprintf("- Skipped source/format count: %d", summary.SkippedTotal))
	if summary.HasDateFilter {
		w(fmt.Sprintf("- Date filter: start `%s`, end `%s`", emptyIfBlank(summary.StartDate), emptyIfBlank(summary.EndDate)))
	} else {
		w("- Date filter: not applied")
	}
	w("")

	w("## Keyword Summary")
	w("")
	for _, term := range summary.Terms {
		w(fmt.Sprintf("- `%s` (`%s`): %d hit(s)", term, TermToDirname(term), summary.KeywordHits[term]))
	}
	if len(summary.RejectedKeywords) > 0 {
		w("- Rejected duplicate keywords:")
		for _, rejected := range summary.RejectedKeywords {
			w(fmt.Sprintf("  - `%s` rejected; kept `%s` for `%s`", rejected.Requested, rejected.Kept, rejected.Normalized))
		}
	}
	w("")

	w("## Dependencies")
	w("")
	writeDependencyLine(w, "readpst", summary.HasReadPST, fmt.Sprintf("PST/OST scanning disabled until `%s` is run", ReadPSTDependencyStatus().InstallHint))
	writeDependencyLine(w, "high-fidelity MSG support", summary.HasHighFidelityMSG, fmt.Sprintf("MSG scanning disabled until `%s` is run", MSGDependencyStatus().InstallHint))
	w("")

	w("## Discovery Summary")
	w("")
	for _, ft := range AllFileTypes() {
		w(fmt.Sprintf("- %s: %d", ft, summary.FileCounts[ft]))
	}
	w("")

	w("## Exception Summary")
	w("")
	w(fmt.Sprintf("- Unknown-date hit count: %d", summary.UnknownDateTotal))
	w(fmt.Sprintf("- Skipped source/format count: %d", summary.SkippedTotal))
	if len(summary.SkippedFormats) > 0 {
		w("- Skipped or disabled formats:")
		for _, item := range summary.SkippedFormats {
			w(fmt.Sprintf("  - %s", item))
		}
	}
	if len(summary.Warnings) > 0 {
		w("- Warnings:")
		for _, item := range summary.Warnings {
			w(fmt.Sprintf("  - %s", item))
		}
	}
	if len(summary.Errors) > 0 {
		w("- Errors:")
		for _, item := range summary.Errors {
			w(fmt.Sprintf("  - %s", item))
		}
	}
	w("")

	w("## Unknown-Date Hits")
	w("")
	if summary.UnknownDateTotal == 0 {
		w("No unknown-date hits were exported.")
	} else {
		for _, term := range summary.Terms {
			if summary.UnknownDateHits[term] == 0 {
				continue
			}
			w(fmt.Sprintf("- `%s`: %d unknown-date hit(s)", term, summary.UnknownDateHits[term]))
		}
	}
	w("")

	w("## Flagged Folders")
	w("")
	if len(summary.FlaggedDirs) == 0 {
		w("No flagged folders detected.")
	} else {
		for _, dir := range summary.FlaggedDirs {
			w(fmt.Sprintf("- `%s`", dir))
		}
	}
	w("")

	w("## Run Artifacts")
	w("")
	w(fmt.Sprintf("- Report: `%s`", RelPath(outputRoot, artifacts.ReportPath)))
	w(fmt.Sprintf("- Reviewer spreadsheet: `%s`", RelPath(outputRoot, artifacts.ReviewWorkbookPath)))
	w(fmt.Sprintf("- Reviewer CSV: `%s`", RelPath(outputRoot, artifacts.ReviewManifestPath)))
	w(fmt.Sprintf("- Technical spreadsheet: `%s`", RelPath(outputRoot, artifacts.ManifestWorkbookPath)))
	w(fmt.Sprintf("- Technical CSV: `%s`", RelPath(outputRoot, artifacts.ManifestPath)))
	w(fmt.Sprintf("- JSON config: `%s`", RelPath(outputRoot, artifacts.ConfigPath)))
	w(fmt.Sprintf("- Log: `%s`", RelPath(outputRoot, filepath.Join(artifacts.RunDir, "script_log.txt"))))
	if len(summary.FlaggedDirs) > 0 {
		w(fmt.Sprintf("- Flagged folders: `%s`", RelPath(outputRoot, filepath.Join(artifacts.RunDir, "flagged_folders.txt"))))
	}
	w("- The per-message hit inventory lives in the reviewer and technical spreadsheet outputs.")
	w("")

	return nil
}

func technicalManifestHeaders() []string {
	return []string{
		"base_name",
		"message_directory_path",
		"source_container_path",
		"source_container_dir",
		"source_type",
		"keyword",
		"keyword_dir",
		"hit_locations",
		"message_relative_path",
		"output_eml_directory_path",
		"output_header_directory_path",
		"status",
		"size_bytes",
		"size_human",
		"note",
	}
}

func technicalManifestRecords(rows []ManifestRow) [][]string {
	records := make([][]string, 0, len(rows))
	for _, row := range rows {
		records = append(records, []string{
			row.MessageBaseName,
			row.MessageDirPath,
			row.SourceContainerPath,
			row.SourceContainerDir,
			row.SourceType,
			row.Keyword,
			row.KeywordDir,
			row.HitLocations,
			row.MessageRelativePath,
			directoryOnlyPath(row.OutputEMLPath),
			directoryOnlyPath(row.OutputHeaderPath),
			row.Status,
			fmt.Sprintf("%d", row.SizeBytes),
			humanSize(row.SizeBytes),
			row.Note,
		})
	}
	return records
}

func reviewManifestHeaders() []string {
	return []string{
		"base_name",
		"message_directory_path",
		"source_container_path",
		"source_type",
		"keyword",
		"hit_locations",
		"exported_message_directory_path",
		"exported_header_directory_path",
		"status",
		"size_bytes",
		"size_human",
		"note",
	}
}

func reviewManifestRecords(rows []ManifestRow) [][]string {
	reviewRows := make([]ManifestRow, 0, len(rows))
	for _, row := range rows {
		if strings.HasPrefix(row.Status, "skipped_") {
			continue
		}
		reviewRows = append(reviewRows, row)
	}
	sort.SliceStable(reviewRows, func(i, j int) bool {
		left, right := reviewRows[i], reviewRows[j]
		leftScore := reviewHitLocationScore(left.HitLocations)
		rightScore := reviewHitLocationScore(right.HitLocations)
		switch {
		case leftScore != rightScore:
			return leftScore > rightScore
		case strings.ToLower(left.Keyword) != strings.ToLower(right.Keyword):
			return strings.ToLower(left.Keyword) < strings.ToLower(right.Keyword)
		case strings.ToLower(left.SourceContainerPath) != strings.ToLower(right.SourceContainerPath):
			return strings.ToLower(left.SourceContainerPath) < strings.ToLower(right.SourceContainerPath)
		default:
			return strings.ToLower(left.MessageRelativePath) < strings.ToLower(right.MessageRelativePath)
		}
	})

	records := make([][]string, 0, len(reviewRows))
	for _, row := range reviewRows {
		records = append(records, []string{
			row.MessageBaseName,
			row.MessageDirPath,
			row.SourceContainerPath,
			row.SourceType,
			row.Keyword,
			reviewHitLocationsLabel(row.HitLocations),
			directoryOnlyPath(row.OutputEMLPath),
			directoryOnlyPath(row.OutputHeaderPath),
			reviewStatusLabel(row.Status),
			fmt.Sprintf("%d", row.SizeBytes),
			humanSize(row.SizeBytes),
			reviewEmailNote(row),
		})
	}
	return records
}

func writeCSV(path string, headers []string, records [][]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write(headers); err != nil {
		return err
	}
	for _, record := range records {
		if err := w.Write(record); err != nil {
			return err
		}
	}
	return w.Error()
}

func writeWorkbook(path, sheetName string, headers []string, records [][]string, tableName string) error {
	f := excelize.NewFile()
	defaultSheet := f.GetSheetName(0)
	if defaultSheet != sheetName {
		f.SetSheetName(defaultSheet, sheetName)
	}
	defer func() { _ = f.Close() }()

	for i, header := range headers {
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			return err
		}
		if err := f.SetCellValue(sheetName, cell, header); err != nil {
			return err
		}
	}
	for rowIndex, record := range records {
		cell, err := excelize.CoordinatesToCellName(1, rowIndex+2)
		if err != nil {
			return err
		}
		values := make([]interface{}, len(record))
		for i, value := range record {
			values[i] = workbookCellValue(headers[i], value)
		}
		if err := f.SetSheetRow(sheetName, cell, &values); err != nil {
			return err
		}
	}
	if err := f.SetPanes(sheetName, &excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      0,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
		Selection:   []excelize.Selection{{SQRef: "A2", ActiveCell: "A2", Pane: "bottomLeft"}},
	}); err != nil {
		return err
	}

	for i, header := range headers {
		width := float64(len(header) + 4)
		for _, record := range records {
			if i < len(record) {
				l := len(record[i]) + 2
				if float64(l) > width {
					width = float64(l)
				}
			}
		}
		if width > 60 {
			width = 60
		}
		if width < 12 {
			width = 12
		}
		col, err := excelize.ColumnNumberToName(i + 1)
		if err != nil {
			return err
		}
		if err := f.SetColWidth(sheetName, col, col, width); err != nil {
			return err
		}
	}

	if len(records) > 0 {
		endCell, err := excelize.CoordinatesToCellName(len(headers), len(records)+1)
		if err != nil {
			return err
		}
		show := true
		if err := f.AddTable(sheetName, &excelize.Table{
			Range:           "A1:" + endCell,
			Name:            tableName,
			StyleName:       "TableStyleMedium2",
			ShowFirstColumn: false,
			ShowLastColumn:  false,
			ShowRowStripes:  &show,
		}); err != nil {
			return err
		}
	}

	return f.SaveAs(path)
}

func workbookCellValue(header, value string) interface{} {
	switch header {
	case "size_bytes":
		if value == "" {
			return nil
		}
		var numeric int64
		if _, err := fmt.Sscanf(value, "%d", &numeric); err == nil {
			return numeric
		}
	}
	return value
}

func writeDependencyLine(w func(string), name string, available bool, note string) {
	if available {
		w(fmt.Sprintf("- %s: available", name))
		return
	}
	w(fmt.Sprintf("- %s: missing", name))
	w(fmt.Sprintf("  - %s", note))
}

func messageDirectoryPath(rel string) string {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if rel == "" || rel == "." {
		return ""
	}
	dir := filepath.ToSlash(filepath.Dir(filepath.FromSlash(rel)))
	if dir == "." {
		return ""
	}
	return dir
}

func directoryOnlyPath(rel string) string {
	return messageDirectoryPath(rel)
}

func messageSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func humanSize(sizeBytes int64) string {
	if sizeBytes <= 0 {
		return ""
	}
	if sizeBytes < 1024 {
		return fmt.Sprintf("%d B", sizeBytes)
	}
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	size := float64(sizeBytes)
	unitIndex := -1
	for size >= 1024 && unitIndex < len(units)-1 {
		size /= 1024
		unitIndex++
	}
	formatted := fmt.Sprintf("%.1f", size)
	formatted = strings.TrimSuffix(formatted, ".0")
	return formatted + " " + units[unitIndex]
}

func reviewHitLocationScore(locations string) int {
	score := 0
	for _, location := range strings.Split(locations, "|") {
		switch strings.TrimSpace(strings.ToLower(location)) {
		case "body":
			score += 4
		case "subject":
			score += 2
		case "header":
			score += 1
		}
	}
	return score
}

func reviewHitLocationsLabel(locations string) string {
	if locations == "" {
		return ""
	}
	parts := strings.Split(locations, "|")
	labels := make([]string, 0, len(parts))
	for _, part := range parts {
		switch strings.TrimSpace(strings.ToLower(part)) {
		case "header":
			labels = append(labels, "Header")
		case "subject":
			labels = append(labels, "Subject")
		case "body":
			labels = append(labels, "Body")
		}
	}
	return strings.Join(labels, ", ")
}

func reviewStatusLabel(status string) string {
	switch status {
	case "export":
		return "exported"
	case "unknown_date":
		return "exported as unknown date"
	case "estimated_export":
		return "estimated export"
	case "estimated_unknown_date":
		return "estimated export as unknown date"
	case "skipped_source":
		return "skipped source"
	case "skipped_dependency":
		return "skipped dependency"
	default:
		return status
	}
}

func reviewEmailNote(row ManifestRow) string {
	parts := make([]string, 0, 3)
	if row.Keyword != "" {
		parts = append(parts, fmt.Sprintf("Matched keyword: %s.", row.Keyword))
	}
	if label := reviewHitLocationsLabel(row.HitLocations); label != "" {
		parts = append(parts, fmt.Sprintf("Hit locations: %s.", label))
	}
	if row.Note != "" {
		parts = append(parts, row.Note)
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func sumKeywordHits(values map[string]int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func cloneEnabledTypes(enabled map[FileType]bool) map[FileType]bool {
	cloned := make(map[FileType]bool, len(enabled))
	for k, v := range enabled {
		cloned[k] = v
	}
	return cloned
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func emptyIfBlank(value string) string {
	if value == "" {
		return "(none)"
	}
	return value
}
