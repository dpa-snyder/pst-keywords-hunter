package filescanner

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"keyword-hunter/scanner"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

var (
	emailExtensions = map[string]bool{
		".pst": true, ".ost": true, ".eml": true, ".msg": true, ".mbox": true, ".mbx": true,
	}
	directTextExtensions = map[string]bool{
		".txt": true, ".csv": true, ".tsv": true, ".rtf": true, ".md": true,
		".json": true, ".xml": true, ".html": true, ".htm": true, ".log": true,
	}
	openXMLExtensions = map[string]bool{
		".docx": true, ".xlsx": true, ".pptx": true,
	}
	sofficeExtensions = map[string]bool{
		".doc": true, ".xls": true, ".ppt": true, ".odt": true, ".ods": true, ".odp": true,
	}
	nestedArchiveExtensions = map[string]bool{
		".zip": true, ".7z": true, ".rar": true, ".tar": true, ".gz": true, ".tgz": true, ".bz2": true, ".xz": true,
	}
	xmlTagPattern = regexp.MustCompile(`<[^>]+>`)
	spacePattern  = regexp.MustCompile(`\s+`)
)

const (
	maxZipMembers                     = 5000
	DefaultMaxZipGB                   = 4
	MinMaxZipGB                       = 1
	MaxMaxZipGB                       = 30
	maxZipMemberContentBytes          = int64(512 * 1024 * 1024)
	maxZipCompressionBombRatio        = 1000
	minZipCompressionBombBytes        = uint64(100 * 1024 * 1024)
	zipEncryptedFlag           uint16 = 0x1
)

type sourceFile struct {
	Path      string
	RelPath   string
	BaseName  string
	Extension string
	SizeBytes int64
	Flagged   bool
}

type classifiedFile struct {
	sourceFile
	ContentSearchable bool
	SearchMethod      FileSearchMethod
	SearchabilityNote string
	DateMetadata      dateMetadata
}

type folderFallbackMatch struct {
	Path      string
	RelPath   string
	BaseName  string
	SizeBytes int64
	Hits      []string
	HitCounts map[string]int
	HitTotal  int
	Flagged   bool
}

type archiveMemberMatch struct {
	InternalPath      string
	BaseName          string
	Extension         string
	SizeBytes         int64
	FilenameHits      []string
	FilenameHitCounts map[string]int
	FilenameHitTotal  int
	ContentHits       []string
	ContentHitCounts  map[string]int
	ContentHitTotal   int
	ContentStatus     string
	ArchiveStatus     string
	Note              string
}

func Run(cfg Config, events chan<- Event) error {
	_, err := RunWithSummary(cfg, events)
	return err
}

func RunWithSummary(cfg Config, events chan<- Event) (*RunSummary, error) {
	return runWithOptionalSnapshot(cfg, events, nil)
}

func RunWithSnapshot(cfg Config, events chan<- Event, snapshot *InventorySnapshot) (*RunSummary, error) {
	return runWithOptionalSnapshot(cfg, events, snapshot)
}

func runWithOptionalSnapshot(cfg Config, events chan<- Event, snapshot *InventorySnapshot) (*RunSummary, error) {
	if events != nil {
		defer close(events)
	}
	searchScope := NormalizeSearchScope(cfg.SearchScope)
	pathSearchEnabled := searchScope.AllowsPathSearch()
	contentSearchEnabled := searchScope.AllowsContentSearch()
	maxZipBytes := NormalizeMaxZipBytes(cfg.MaxZipBytes)
	scanner.PrepareMacOutputTree()
	if err := scanner.ValidateRoots(cfg.SourceDir, cfg.OutputDir); err != nil {
		return nil, err
	}
	info, err := os.Stat(cfg.SourceDir)
	if err != nil {
		return nil, fmt.Errorf("failed to access source directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("source path must be a directory")
	}

	var prescan *PrescanResult
	var files []classifiedFile
	usedSnapshot := false
	if snapshot != nil && snapshot.MatchesSource(cfg.SourceDir) {
		prescan = clonePrescanResult(snapshot.prescan)
		files = append([]classifiedFile(nil), snapshot.files...)
		usedSnapshot = true
		if events != nil {
			events <- Event{
				Type:                  EventDiscovery,
				Message:               fmt.Sprintf("Using prescan snapshot: %d non-email file(s)", len(files)),
				TotalFiles:            len(files),
				UsedInventorySnapshot: true,
			}
		}
	} else {
		var err error
		prescan, files, err = discoverAndClassifyForRun(cfg.SourceDir, events)
		if err != nil {
			return nil, err
		}
	}

	runTimestamp := scanner.TimestampForFilename()
	runDir := filepath.Join(cfg.OutputDir, runTimestamp)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create run directory: %w", err)
	}
	defer scanner.CleanupAppleDoubleArtifacts(runDir)

	logger, err := scanner.NewLogger(runDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}
	defer logger.Close()

	mode := "scan"
	if cfg.DryRun {
		mode = "estimate"
	}
	summary := &RunSummary{
		RunTimestamp:            runTimestamp,
		RunDir:                  filepath.Base(runDir),
		ReportPath:              filepath.Join(runDir, fmt.Sprintf("run_report_%s.md", runTimestamp)),
		ManifestPath:            filepath.Join(runDir, fmt.Sprintf("match_manifest_%s.csv", runTimestamp)),
		ReviewManifestPath:      filepath.Join(runDir, fmt.Sprintf("review_manifest_%s.csv", runTimestamp)),
		ManifestWorkbookPath:    filepath.Join(runDir, fmt.Sprintf("match_manifest_%s.xlsx", runTimestamp)),
		ReviewWorkbookPath:      filepath.Join(runDir, fmt.Sprintf("review_manifest_%s.xlsx", runTimestamp)),
		InventoryPath:           filepath.Join(runDir, fmt.Sprintf("inventory_%s.csv", runTimestamp)),
		ConfigPath:              filepath.Join(runDir, fmt.Sprintf("run_config_%s.json", runTimestamp)),
		LogPath:                 filepath.Join(runDir, "script_log.txt"),
		Mode:                    mode,
		SearchScope:             searchScope,
		MaxMatches:              cfg.MaxMatches,
		MaxContentBytes:         cfg.MaxContentBytes,
		MaxZipBytes:             maxZipBytes,
		UsedInventorySnapshot:   usedSnapshot,
		HasDateFilter:           cfg.StartDate != nil || cfg.EndDate != nil,
		DatePolicy:              "Files with a best available date before the start of the review range are excluded. Files with no usable date or a best available date after the review range are treated as unknown.",
		SourceRootLabel:         filepath.Base(cfg.SourceDir),
		OutputRootLabel:         filepath.Base(cfg.OutputDir),
		Terms:                   append([]string(nil), cfg.Terms...),
		RejectedKeywords:        append([]scanner.RejectedKeyword(nil), cfg.RejectedKeywords...),
		Dependencies:            prescan.Dependencies,
		FilesDiscovered:         prescan.FilesDiscovered,
		IgnoredEmailFiles:       prescan.IgnoredEmailFiles,
		TotalBytes:              prescan.TotalBytes,
		ScanBytes:               prescan.ScanBytes,
		IgnoredEmailBytes:       prescan.IgnoredEmailBytes,
		ContentSearchableFiles:  prescan.ContentSearchableFiles,
		ContentSearchableBytes:  prescan.ContentSearchableBytes,
		FilenameOnlyFiles:       prescan.FilenameOnlyFiles,
		FilenameOnlyBytes:       prescan.FilenameOnlyBytes,
		FilenameHitsByKeyword:   make(map[string]int),
		FolderHitsByKeyword:     make(map[string]int),
		ContentHitsByKeyword:    make(map[string]int),
		InRangeMatchedFiles:     0,
		UnknownDateMatchedFiles: 0,
		ExcludedPreRangeFiles:   0,
		Warnings:                []string{},
		Errors:                  []string{},
		FlaggedDirs:             append([]string(nil), prescan.FlaggedDirs...),
		ExtensionStats:          append([]ExtensionStat(nil), prescan.ExtensionStats...),
		InventoryRows:           make([]InventoryRow, 0, len(files)),
		ManifestRows:            []MatchRow{},
	}

	logger.Log(fmt.Sprintf("=== File Keyword Hunter %s started at %s ===", strings.Title(mode), scanner.Timestamp()))
	logger.Log(fmt.Sprintf("Source root: %s", summary.SourceRootLabel))
	logger.Log(fmt.Sprintf("Output root: %s", summary.OutputRootLabel))
	logger.Log(fmt.Sprintf("Run directory: %s", summary.RunDir))
	logger.Log(fmt.Sprintf("Search scope: %s", searchScope))
	if usedSnapshot {
		logger.Log("Prescan snapshot: used")
	} else {
		logger.Log("Prescan snapshot: not used")
	}
	if cfg.MaxMatches > 0 {
		logger.Log(fmt.Sprintf("Max matched items: %d", cfg.MaxMatches))
	}
	if cfg.MaxContentBytes > 0 {
		logger.Log(fmt.Sprintf("Max content extraction bytes: %d", cfg.MaxContentBytes))
	}
	logger.Log(fmt.Sprintf("Max ZIP uncompressed bytes: %d", maxZipBytes))
	logger.Log(fmt.Sprintf("Non-email files discovered: %d", summary.FilesDiscovered-summary.IgnoredEmailFiles))
	logger.Log(fmt.Sprintf("Ignored email files: %d", summary.IgnoredEmailFiles))
	if len(summary.FlaggedDirs) > 0 {
		logger.Log(fmt.Sprintf("Flagged folders: %d", len(summary.FlaggedDirs)))
	}
	if len(cfg.Terms) > 0 {
		logger.Log("Terms: " + strings.Join(cfg.Terms, ", "))
	}
	if summary.HasDateFilter {
		if cfg.StartDate != nil {
			summary.StartDate = cfg.StartDate.Format("2006-01-02")
		}
		if cfg.EndDate != nil {
			summary.EndDate = cfg.EndDate.Format("2006-01-02")
		}
		logger.Log(fmt.Sprintf("Date policy: start=%s end=%s; pre-range files excluded, missing or post-range dates treated as unknown", emptyIfBlank(summary.StartDate), emptyIfBlank(summary.EndDate)))
	}
	logger.Log("")

	if events != nil {
		events <- Event{
			Type:                  EventDiscovery,
			Message:               fmt.Sprintf("Discovered %d non-email file(s)", len(files)),
			TotalFiles:            len(files),
			MatchedFiles:          0,
			CopiedFiles:           0,
			UsedInventorySnapshot: usedSnapshot,
		}
	}

	copyDir := filepath.Join(runDir, "matched_files")
	if !cfg.DryRun {
		if err := os.MkdirAll(copyDir, 0755); err != nil {
			return nil, err
		}
	}

	filenameMatchedDirs := map[string]bool{}
	copiedSources := map[string]string{}
	for idx, file := range files {
		if maxMatchesReached(summary, cfg.MaxMatches) {
			summary.StoppedByMaxMatches = true
			msg := fmt.Sprintf("Stopped after reaching max matched items: %d", cfg.MaxMatches)
			summary.Warnings = append(summary.Warnings, msg)
			logger.Log("STOP: " + msg)
			break
		}
		summary.FilesScanned++
		row := InventoryRow{
			SourceRelativePath: file.RelPath,
			BaseName:           file.BaseName,
			Extension:          displayExt(file.Extension),
			SizeBytes:          file.SizeBytes,
			ContentSearchable:  file.ContentSearchable,
			SearchMethod:       string(file.SearchMethod),
			SearchabilityNote:  file.SearchabilityNote,
			FlaggedParent:      file.Flagged,
		}
		summary.InventoryRows = append(summary.InventoryRows, row)

		if events != nil {
			events <- Event{
				Type:          EventFileStart,
				SourceFile:    file.RelPath,
				Message:       fmt.Sprintf("[%04d] %s", idx+1, file.RelPath),
				FileNum:       idx + 1,
				TotalFiles:    len(files),
				MatchedFiles:  summary.MatchedFiles,
				CopiedFiles:   summary.CopiedFiles,
				SearchMethod:  string(file.SearchMethod),
				Searchability: file.SearchabilityNote,
			}
		}

		filenameHits := []string{}
		filenameHitCounts := map[string]int{}
		filenameHitTotal := 0
		if pathSearchEnabled {
			filenameHits, filenameHitCounts, filenameHitTotal = findHits(strings.ToLower(file.BaseName), cfg.Terms)
			if len(filenameHits) > 0 {
				markAncestorDirs(filenameMatchedDirs, cfg.SourceDir, filepath.Dir(file.Path))
			}
		}
		contentHits := []string{}
		contentHitCounts := map[string]int{}
		contentHitTotal := 0
		note := file.SearchabilityNote

		if file.SearchMethod == MethodZipArchive {
			dateMeta := determineDateMetadata(file, cfg.StartDate, cfg.EndDate)
			if dateMeta.Status == DateStatusExcludedPreRange {
				summary.ExcludedPreRangeFiles++
				logger.Log(fmt.Sprintf("SKIP DATE RANGE ARCHIVE: %s", file.RelPath))
				if events != nil {
					events <- Event{
						Type:         EventFileDone,
						SourceFile:   file.RelPath,
						FileNum:      idx + 1,
						TotalFiles:   len(files),
						MatchedFiles: summary.MatchedFiles,
						CopiedFiles:  summary.CopiedFiles,
						Message:      fmt.Sprintf("Skipped pre-range archive: %s", file.RelPath),
						Note:         dateMeta.StatusNote,
					}
				}
				continue
			}

			archiveMatches, archiveWarnings := scanZipArchive(file, cfg.Terms, pathSearchEnabled, contentSearchEnabled, cfg.MaxContentBytes, maxZipBytes)
			for _, warning := range archiveWarnings {
				warn := fmt.Sprintf("%s: %s", file.RelPath, warning)
				summary.Warnings = append(summary.Warnings, warn)
				logger.Log("WARNING: " + warn)
			}

			if len(filenameHits) == 0 && len(archiveMatches) == 0 {
				if events != nil {
					events <- Event{
						Type:         EventFileDone,
						SourceFile:   file.RelPath,
						FileNum:      idx + 1,
						TotalFiles:   len(files),
						MatchedFiles: summary.MatchedFiles,
						CopiedFiles:  summary.CopiedFiles,
						Message:      fmt.Sprintf("Done: %s", file.RelPath),
					}
				}
				continue
			}

			copiedPath := ""
			if !cfg.DryRun {
				var copiedNow bool
				var copyErr error
				copiedPath, copiedNow, copyErr = copySourceFileOnce(copiedSources, file.Path, file.RelPath, cfg.OutputDir, copyDir)
				if copyErr != nil {
					summary.Errors = append(summary.Errors, fmt.Sprintf("%s: failed to copy archive match: %v", file.RelPath, copyErr))
				} else if copiedNow {
					summary.CopiedFiles++
				}
			}

			if len(filenameHits) > 0 && !maxMatchesReached(summary, cfg.MaxMatches) {
				for _, term := range filenameHits {
					summary.FilenameHitsByKeyword[term] += filenameHitCounts[term]
				}
				filenameTotal, folderTotal, contentTotal := hitCategoryTotals(summary)
				summary.MatchedFiles++
				if dateMeta.Status == DateStatusInRange {
					summary.InRangeMatchedFiles++
				} else {
					summary.UnknownDateMatchedFiles++
				}
				match := MatchRow{
					SourceRelativePath: file.RelPath,
					BaseName:           file.BaseName,
					Extension:          displayExt(file.Extension),
					FilenameHits:       filenameHits,
					FilenameHitCounts:  filenameHitCounts,
					FilenameHitTotal:   filenameHitTotal,
					FolderHits:         []string{},
					FolderHitCounts:    map[string]int{},
					FolderHitTotal:     0,
					ContentHits:        []string{},
					ContentHitCounts:   map[string]int{},
					ContentHitTotal:    0,
					ArchivePath:        file.RelPath,
					ArchiveStatus:      "container_filename_match",
					DocumentDate:       formatOptionalTime(dateMeta.Selected),
					DocumentDateSource: dateMeta.SelectedSource,
					DateStatus:         string(dateMeta.Status),
					DateNote:           dateMeta.StatusNote,
					FilesystemCreated:  formatOptionalTime(dateMeta.FilesystemCreated),
					FilesystemModified: formatOptionalTime(dateMeta.FilesystemModified),
					EmbeddedCreated:    formatOptionalTime(dateMeta.EmbeddedCreated),
					EmbeddedModified:   formatOptionalTime(dateMeta.EmbeddedModified),
					ContentStatus:      "archive_container",
					CopiedFilePath:     copiedPath,
					Note:               strings.TrimSpace(strings.Trim(strings.Join([]string{"ZIP filename match; internal members scanned separately", relativeDateNote(dateMeta.Status, dateMeta.SelectedSource)}, " | "), " |")),
					SizeBytes:          file.SizeBytes,
				}
				summary.ManifestRows = append(summary.ManifestRows, match)
				logger.Log(fmt.Sprintf("MATCH: %s", file.RelPath))
				if events != nil {
					events <- Event{
						Type:                  EventMatch,
						SourceFile:            file.RelPath,
						Message:               fmt.Sprintf("Match: %s", file.RelPath),
						FileNum:               idx + 1,
						TotalFiles:            len(files),
						MatchedFiles:          summary.MatchedFiles,
						CopiedFiles:           summary.CopiedFiles,
						FilenameHits:          filenameTotal,
						FolderHits:            folderTotal,
						ContentHits:           contentTotal,
						KeywordHitStats:       keywordHitStats(summary),
						UsedInventorySnapshot: usedSnapshot,
						OutputPath:            copiedPath,
						SearchMethod:          string(file.SearchMethod),
						Note:                  note,
					}
				}
			}

			for _, archiveMatch := range archiveMatches {
				if maxMatchesReached(summary, cfg.MaxMatches) {
					summary.StoppedByMaxMatches = true
					msg := fmt.Sprintf("Stopped archive member scan after reaching max matched items: %d", cfg.MaxMatches)
					summary.Warnings = append(summary.Warnings, msg)
					logger.Log("STOP: " + msg)
					break
				}
				for _, term := range archiveMatch.FilenameHits {
					summary.FilenameHitsByKeyword[term] += archiveMatch.FilenameHitCounts[term]
				}
				for _, term := range archiveMatch.ContentHits {
					summary.ContentHitsByKeyword[term] += archiveMatch.ContentHitCounts[term]
				}
				filenameTotal, folderTotal, contentTotal := hitCategoryTotals(summary)
				summary.MatchedFiles++
				if dateMeta.Status == DateStatusInRange {
					summary.InRangeMatchedFiles++
				} else {
					summary.UnknownDateMatchedFiles++
				}
				memberSourcePath := file.RelPath + " :: " + archiveMatch.InternalPath
				match := MatchRow{
					SourceRelativePath:  memberSourcePath,
					BaseName:            archiveMatch.BaseName,
					Extension:           displayExt(archiveMatch.Extension),
					FilenameHits:        archiveMatch.FilenameHits,
					FilenameHitCounts:   archiveMatch.FilenameHitCounts,
					FilenameHitTotal:    archiveMatch.FilenameHitTotal,
					FolderHits:          []string{},
					FolderHitCounts:     map[string]int{},
					FolderHitTotal:      0,
					ContentHits:         archiveMatch.ContentHits,
					ContentHitCounts:    archiveMatch.ContentHitCounts,
					ContentHitTotal:     archiveMatch.ContentHitTotal,
					ArchivePath:         file.RelPath,
					ArchiveInternalPath: archiveMatch.InternalPath,
					ArchiveStatus:       archiveMatch.ArchiveStatus,
					DocumentDate:        formatOptionalTime(dateMeta.Selected),
					DocumentDateSource:  dateMeta.SelectedSource,
					DateStatus:          string(dateMeta.Status),
					DateNote:            dateMeta.StatusNote,
					FilesystemCreated:   formatOptionalTime(dateMeta.FilesystemCreated),
					FilesystemModified:  formatOptionalTime(dateMeta.FilesystemModified),
					EmbeddedCreated:     formatOptionalTime(dateMeta.EmbeddedCreated),
					EmbeddedModified:    formatOptionalTime(dateMeta.EmbeddedModified),
					ContentStatus:       archiveMatch.ContentStatus,
					CopiedFilePath:      copiedPath,
					Note:                strings.TrimSpace(strings.Trim(strings.Join([]string{archiveMatch.Note, relativeDateNote(dateMeta.Status, dateMeta.SelectedSource)}, " | "), " |")),
					SizeBytes:           archiveMatch.SizeBytes,
				}
				summary.ManifestRows = append(summary.ManifestRows, match)
				logger.Log(fmt.Sprintf("ARCHIVE MATCH: %s", memberSourcePath))
				if events != nil {
					events <- Event{
						Type:                  EventMatch,
						SourceFile:            memberSourcePath,
						Message:               fmt.Sprintf("Archive match: %s", memberSourcePath),
						FileNum:               idx + 1,
						TotalFiles:            len(files),
						MatchedFiles:          summary.MatchedFiles,
						CopiedFiles:           summary.CopiedFiles,
						FilenameHits:          filenameTotal,
						FolderHits:            folderTotal,
						ContentHits:           contentTotal,
						KeywordHitStats:       keywordHitStats(summary),
						UsedInventorySnapshot: usedSnapshot,
						OutputPath:            copiedPath,
						SearchMethod:          string(file.SearchMethod),
						Note:                  archiveMatch.Note,
					}
				}
			}

			if events != nil {
				events <- Event{
					Type:         EventFileDone,
					SourceFile:   file.RelPath,
					FileNum:      idx + 1,
					TotalFiles:   len(files),
					MatchedFiles: summary.MatchedFiles,
					CopiedFiles:  summary.CopiedFiles,
					Message:      fmt.Sprintf("Done: %s", file.RelPath),
				}
			}
			continue
		}

		if contentSearchEnabled && file.ContentSearchable {
			if cfg.MaxContentBytes > 0 && file.SizeBytes > cfg.MaxContentBytes {
				summary.ContentSizeSkippedFiles++
				note = fmt.Sprintf("content extraction skipped; file size %d exceeds max content bytes %d", file.SizeBytes, cfg.MaxContentBytes)
			} else {
				text, extractErr := extractSearchText(file)
				if extractErr != nil {
					warn := fmt.Sprintf("%s: %v", file.RelPath, extractErr)
					summary.Warnings = append(summary.Warnings, warn)
					logger.Log("WARNING: " + warn)
					note = "content extraction failed; filename search only"
				} else {
					contentHits, contentHitCounts, contentHitTotal = findHits(text, cfg.Terms)
				}
			}
		}

		if len(filenameHits) == 0 && len(contentHits) == 0 {
			if events != nil {
				events <- Event{
					Type:         EventFileDone,
					SourceFile:   file.RelPath,
					FileNum:      idx + 1,
					TotalFiles:   len(files),
					MatchedFiles: summary.MatchedFiles,
					CopiedFiles:  summary.CopiedFiles,
					Message:      fmt.Sprintf("Done: %s", file.RelPath),
				}
			}
			continue
		}

		dateMeta := determineDateMetadata(file, cfg.StartDate, cfg.EndDate)
		if dateMeta.Status == DateStatusExcludedPreRange {
			summary.ExcludedPreRangeFiles++
			logger.Log(fmt.Sprintf("SKIP DATE RANGE: %s", file.RelPath))
			if events != nil {
				events <- Event{
					Type:         EventFileDone,
					SourceFile:   file.RelPath,
					FileNum:      idx + 1,
					TotalFiles:   len(files),
					MatchedFiles: summary.MatchedFiles,
					CopiedFiles:  summary.CopiedFiles,
					Message:      fmt.Sprintf("Skipped pre-range: %s", file.RelPath),
					Note:         dateMeta.StatusNote,
				}
			}
			continue
		}

		copiedPath := ""
		if !cfg.DryRun {
			var copiedNow bool
			var copyErr error
			copiedPath, copiedNow, copyErr = copySourceFileOnce(copiedSources, file.Path, file.RelPath, cfg.OutputDir, copyDir)
			if copyErr != nil {
				summary.Errors = append(summary.Errors, fmt.Sprintf("%s: failed to copy match: %v", file.RelPath, copyErr))
			} else if copiedNow {
				summary.CopiedFiles++
			}
		}

		for _, term := range filenameHits {
			summary.FilenameHitsByKeyword[term] += filenameHitCounts[term]
		}
		for _, term := range contentHits {
			summary.ContentHitsByKeyword[term] += contentHitCounts[term]
		}
		filenameTotal, folderTotal, contentTotal := hitCategoryTotals(summary)
		summary.MatchedFiles++
		if dateMeta.Status == DateStatusInRange {
			summary.InRangeMatchedFiles++
		} else {
			summary.UnknownDateMatchedFiles++
		}
		match := MatchRow{
			SourceRelativePath: file.RelPath,
			BaseName:           file.BaseName,
			Extension:          displayExt(file.Extension),
			FilenameHits:       filenameHits,
			FilenameHitCounts:  filenameHitCounts,
			FilenameHitTotal:   filenameHitTotal,
			FolderHits:         []string{},
			FolderHitCounts:    map[string]int{},
			FolderHitTotal:     0,
			ContentHits:        contentHits,
			ContentHitCounts:   contentHitCounts,
			ContentHitTotal:    contentHitTotal,
			DocumentDate:       formatOptionalTime(dateMeta.Selected),
			DocumentDateSource: dateMeta.SelectedSource,
			DateStatus:         string(dateMeta.Status),
			DateNote:           dateMeta.StatusNote,
			FilesystemCreated:  formatOptionalTime(dateMeta.FilesystemCreated),
			FilesystemModified: formatOptionalTime(dateMeta.FilesystemModified),
			EmbeddedCreated:    formatOptionalTime(dateMeta.EmbeddedCreated),
			EmbeddedModified:   formatOptionalTime(dateMeta.EmbeddedModified),
			ContentStatus:      contentStatus(file, note),
			CopiedFilePath:     copiedPath,
			Note:               strings.TrimSpace(strings.Trim(strings.Join([]string{note, relativeDateNote(dateMeta.Status, dateMeta.SelectedSource)}, " | "), " |")),
			SizeBytes:          file.SizeBytes,
		}
		summary.ManifestRows = append(summary.ManifestRows, match)
		logger.Log(fmt.Sprintf("MATCH: %s", file.RelPath))

		if events != nil {
			events <- Event{
				Type:                  EventMatch,
				SourceFile:            file.RelPath,
				Message:               fmt.Sprintf("Match: %s", file.RelPath),
				FileNum:               idx + 1,
				TotalFiles:            len(files),
				MatchedFiles:          summary.MatchedFiles,
				CopiedFiles:           summary.CopiedFiles,
				FilenameHits:          filenameTotal,
				FolderHits:            folderTotal,
				ContentHits:           contentTotal,
				KeywordHitStats:       keywordHitStats(summary),
				UsedInventorySnapshot: usedSnapshot,
				OutputPath:            copiedPath,
				SearchMethod:          string(file.SearchMethod),
				Note:                  note,
			}
		}

		if events != nil {
			events <- Event{
				Type:         EventFileDone,
				SourceFile:   file.RelPath,
				FileNum:      idx + 1,
				TotalFiles:   len(files),
				MatchedFiles: summary.MatchedFiles,
				CopiedFiles:  summary.CopiedFiles,
				Message:      fmt.Sprintf("Done: %s", file.RelPath),
			}
		}
	}

	if pathSearchEnabled && !maxMatchesReached(summary, cfg.MaxMatches) {
		folderFallbacks := findFolderFallbackMatches(cfg.SourceDir, files, cfg.Terms, filenameMatchedDirs)
		for _, folder := range folderFallbacks {
			if maxMatchesReached(summary, cfg.MaxMatches) {
				summary.StoppedByMaxMatches = true
				msg := fmt.Sprintf("Stopped folder fallback after reaching max matched items: %d", cfg.MaxMatches)
				summary.Warnings = append(summary.Warnings, msg)
				logger.Log("STOP: " + msg)
				break
			}
			pseudoFile := classifiedFile{
				sourceFile: sourceFile{
					Path:      folder.Path,
					RelPath:   folder.RelPath,
					BaseName:  folder.BaseName,
					Extension: "",
					SizeBytes: folder.SizeBytes,
					Flagged:   folder.Flagged,
				},
				ContentSearchable: false,
				SearchMethod:      MethodFilenameOnly,
				SearchabilityNote: "folder-name fallback; folder subtree has no content-searchable files and no filename hits",
			}
			dateMeta := determineDateMetadata(pseudoFile, cfg.StartDate, cfg.EndDate)
			if dateMeta.Status == DateStatusExcludedPreRange {
				summary.ExcludedPreRangeFiles++
				logger.Log(fmt.Sprintf("SKIP DATE RANGE FOLDER: %s", folder.RelPath))
				continue
			}

			copiedPath := ""
			if !cfg.DryRun {
				destPath, err := sourceRelativeOutputPath(copyDir, folder.RelPath)
				if err != nil {
					summary.Errors = append(summary.Errors, fmt.Sprintf("%s: failed to prepare folder copy path: %v", folder.RelPath, err))
				} else if err := copyDirTree(folder.Path, destPath); err != nil {
					summary.Errors = append(summary.Errors, fmt.Sprintf("%s: failed to copy folder match: %v", folder.RelPath, err))
				} else {
					copiedPath = scanner.RelPath(cfg.OutputDir, destPath)
					summary.CopiedFiles++
				}
			}

			for _, term := range folder.Hits {
				summary.FolderHitsByKeyword[term] += folder.HitCounts[term]
			}
			filenameTotal, folderTotal, contentTotal := hitCategoryTotals(summary)
			summary.MatchedFiles++
			if dateMeta.Status == DateStatusInRange {
				summary.InRangeMatchedFiles++
			} else {
				summary.UnknownDateMatchedFiles++
			}

			match := MatchRow{
				SourceRelativePath: folder.RelPath,
				BaseName:           folder.BaseName,
				Extension:          "[folder]",
				FilenameHits:       []string{},
				FilenameHitCounts:  map[string]int{},
				FilenameHitTotal:   0,
				FolderHits:         folder.Hits,
				FolderHitCounts:    folder.HitCounts,
				FolderHitTotal:     folder.HitTotal,
				ContentHits:        []string{},
				ContentHitCounts:   map[string]int{},
				ContentHitTotal:    0,
				DocumentDate:       formatOptionalTime(dateMeta.Selected),
				DocumentDateSource: dateMeta.SelectedSource,
				DateStatus:         string(dateMeta.Status),
				DateNote:           dateMeta.StatusNote,
				FilesystemCreated:  formatOptionalTime(dateMeta.FilesystemCreated),
				FilesystemModified: formatOptionalTime(dateMeta.FilesystemModified),
				EmbeddedCreated:    formatOptionalTime(dateMeta.EmbeddedCreated),
				EmbeddedModified:   formatOptionalTime(dateMeta.EmbeddedModified),
				ContentStatus:      "folder_fallback",
				CopiedFilePath:     copiedPath,
				Note:               strings.TrimSpace(strings.Trim(strings.Join([]string{pseudoFile.SearchabilityNote, relativeDateNote(dateMeta.Status, dateMeta.SelectedSource)}, " | "), " |")),
				SizeBytes:          folder.SizeBytes,
			}
			summary.ManifestRows = append(summary.ManifestRows, match)
			logger.Log(fmt.Sprintf("FOLDER MATCH: %s", folder.RelPath))

			if events != nil {
				events <- Event{
					Type:                  EventMatch,
					SourceFile:            folder.RelPath,
					Message:               fmt.Sprintf("Folder match: %s", folder.RelPath),
					TotalFiles:            len(files),
					MatchedFiles:          summary.MatchedFiles,
					CopiedFiles:           summary.CopiedFiles,
					FilenameHits:          filenameTotal,
					FolderHits:            folderTotal,
					ContentHits:           contentTotal,
					KeywordHitStats:       keywordHitStats(summary),
					UsedInventorySnapshot: usedSnapshot,
					OutputPath:            copiedPath,
					SearchMethod:          "folder_fallback",
					Note:                  pseudoFile.SearchabilityNote,
				}
			}
		}
	}

	if err := writeInventory(summary.InventoryPath, summary.InventoryRows); err != nil {
		return nil, err
	}
	if err := writeManifest(summary.ManifestPath, summary.ManifestRows); err != nil {
		return nil, err
	}
	if err := writeManifestWorkbook(summary.ManifestWorkbookPath, summary.ManifestRows); err != nil {
		return nil, err
	}
	if err := writeReviewManifest(summary.ReviewManifestPath, summary.ManifestRows); err != nil {
		return nil, err
	}
	if err := writeReviewWorkbook(summary.ReviewWorkbookPath, summary.ManifestRows); err != nil {
		return nil, err
	}
	if err := writeConfig(summary.ConfigPath, summary, cfg); err != nil {
		return nil, err
	}
	if err := writeReport(summary.ReportPath, summary); err != nil {
		return nil, err
	}
	_ = scanner.CleanupAppleDoubleArtifacts(runDir)

	if events != nil {
		filenameTotal, folderTotal, contentTotal := hitCategoryTotals(summary)
		events <- Event{
			Type:                  EventComplete,
			Message:               fmt.Sprintf("%s complete. Report: %s", strings.Title(mode), scanner.RelPath(cfg.OutputDir, summary.ReportPath)),
			TotalFiles:            len(files),
			MatchedFiles:          summary.MatchedFiles,
			CopiedFiles:           summary.CopiedFiles,
			FilenameHits:          filenameTotal,
			FolderHits:            folderTotal,
			ContentHits:           contentTotal,
			KeywordHitStats:       keywordHitStats(summary),
			UsedInventorySnapshot: usedSnapshot,
		}
	}

	return summary, nil
}

func Prescan(sourceDir string) (*PrescanResult, error) {
	return PrescanWithContext(context.Background(), sourceDir)
}

func PrescanWithContext(ctx context.Context, sourceDir string) (*PrescanResult, error) {
	snapshot, err := BuildInventorySnapshotWithContext(ctx, sourceDir)
	if err != nil {
		return nil, err
	}
	return snapshot.PrescanResult(), nil
}

func BuildInventorySnapshot(sourceDir string) (*InventorySnapshot, error) {
	return BuildInventorySnapshotWithContext(context.Background(), sourceDir)
}

func BuildInventorySnapshotWithContext(ctx context.Context, sourceDir string) (*InventorySnapshot, error) {
	prescan, files, err := discoverAndClassifyWithContext(ctx, sourceDir)
	if err != nil {
		return nil, err
	}
	return newInventorySnapshot(sourceDir, prescan, files), nil
}

func BuildInventorySnapshotWithContextAndEvents(ctx context.Context, sourceDir string, prescanEvents chan<- PrescanProgress) (*InventorySnapshot, error) {
	prescan, files, err := discoverAndClassifyWithContextAndPrescanEvents(ctx, sourceDir, prescanEvents)
	if err != nil {
		return nil, err
	}
	return newInventorySnapshot(sourceDir, prescan, files), nil
}

func discoverAndClassify(sourceDir string) (*PrescanResult, []classifiedFile, error) {
	return discoverAndClassifyWithContext(context.Background(), sourceDir)
}

func discoverAndClassifyForRun(sourceDir string, events chan<- Event) (*PrescanResult, []classifiedFile, error) {
	return discoverAndClassifyWithContextAndEvents(context.Background(), sourceDir, events)
}

func discoverAndClassifyWithContext(ctx context.Context, sourceDir string) (*PrescanResult, []classifiedFile, error) {
	return discoverAndClassifyWithContextAndEvents(ctx, sourceDir, nil)
}

func discoverAndClassifyWithContextAndPrescanEvents(ctx context.Context, sourceDir string, prescanEvents chan<- PrescanProgress) (*PrescanResult, []classifiedFile, error) {
	return discoverAndClassifyInternal(ctx, sourceDir, nil, prescanEvents)
}

func discoverAndClassifyWithContextAndEvents(ctx context.Context, sourceDir string, events chan<- Event) (*PrescanResult, []classifiedFile, error) {
	return discoverAndClassifyInternal(ctx, sourceDir, events, nil)
}

func discoverAndClassifyInternal(ctx context.Context, sourceDir string, events chan<- Event, prescanEvents chan<- PrescanProgress) (*PrescanResult, []classifiedFile, error) {
	flaggedSet := map[string]bool{}
	extensionCounts := map[string]*ExtensionStat{}
	dependencies := DependencyStatuses()
	var files []classifiedFile
	dateMetadataCached := 0
	prescan := &PrescanResult{
		SourceDir:      sourceDir,
		FlaggedDirs:    []string{},
		Dependencies:   dependencies,
		ExtensionStats: []ExtensionStat{},
	}

	emitPrescanProgress := func(stage, message, currentFile string) {
		if prescanEvents == nil {
			return
		}
		prescanEvents <- PrescanProgress{
			Stage:                  stage,
			Message:                message,
			CurrentFile:            currentFile,
			FilesDiscovered:        prescan.FilesDiscovered,
			IgnoredEmailFiles:      prescan.IgnoredEmailFiles,
			ContentSearchableFiles: prescan.ContentSearchableFiles,
			FilenameOnlyFiles:      prescan.FilenameOnlyFiles,
			TotalBytes:             prescan.TotalBytes,
			ScanBytes:              prescan.ScanBytes,
			DateMetadataCached:     dateMetadataCached,
		}
	}

	emitPrescanProgress("starting", "Starting prescan inventory and per-file date cache…", "")

	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if path != sourceDir && scanner.ShouldSkipWalkDir(path) {
				return filepath.SkipDir
			}
			if path != sourceDir && scanner.IsFlaggedFolderName(info.Name()) {
				flaggedSet[path] = true
			}
			return nil
		}
		if scanner.ShouldSkipFileName(info.Name()) {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(info.Name()))
		prescan.TotalBytes += info.Size()
		prescan.FilesDiscovered++
		if events != nil && prescan.FilesDiscovered%1000 == 0 {
			events <- Event{
				Type:       EventDiscovery,
				Message:    fmt.Sprintf("Discovering source: %d file(s) seen", prescan.FilesDiscovered),
				TotalFiles: prescan.FilesDiscovered,
			}
		}
		if prescan.FilesDiscovered == 1 || prescan.FilesDiscovered%250 == 0 {
			emitPrescanProgress(
				"walking",
				fmt.Sprintf("Prescanning source: %d file(s) inventoried, %d date records cached", prescan.FilesDiscovered, dateMetadataCached),
				scanner.RelPath(sourceDir, path),
			)
		}

		stat := extensionCounts[ext]
		if stat == nil {
			stat = &ExtensionStat{Extension: displayExt(ext)}
			extensionCounts[ext] = stat
		}
		stat.Count++
		stat.SizeBytes += info.Size()

		if emailExtensions[ext] {
			prescan.IgnoredEmailFiles++
			prescan.IgnoredEmailBytes += info.Size()
			return nil
		}

		source := sourceFile{
			Path:      path,
			RelPath:   scanner.RelPath(sourceDir, path),
			BaseName:  filepath.Base(path),
			Extension: ext,
			SizeBytes: info.Size(),
			Flagged:   hasFlaggedAncestor(path, flaggedSet),
		}
		classified := classify(source)
		classified.DateMetadata = collectDateMetadata(classified, info)
		dateMetadataCached++
		files = append(files, classified)
		prescan.ScanBytes += info.Size()
		if classified.ContentSearchable {
			prescan.ContentSearchableFiles++
			prescan.ContentSearchableBytes += info.Size()
		} else {
			prescan.FilenameOnlyFiles++
			prescan.FilenameOnlyBytes += info.Size()
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	for dir := range flaggedSet {
		prescan.FlaggedDirs = append(prescan.FlaggedDirs, scanner.RelPath(sourceDir, dir))
	}
	sort.Strings(prescan.FlaggedDirs)

	for _, stat := range extensionCounts {
		prescan.ExtensionStats = append(prescan.ExtensionStats, *stat)
	}
	sort.Slice(prescan.ExtensionStats, func(i, j int) bool {
		if prescan.ExtensionStats[i].Count == prescan.ExtensionStats[j].Count {
			return prescan.ExtensionStats[i].Extension < prescan.ExtensionStats[j].Extension
		}
		return prescan.ExtensionStats[i].Count > prescan.ExtensionStats[j].Count
	})
	prescan.TopExtensionStats = append(prescan.TopExtensionStats, prescan.ExtensionStats...)
	if len(prescan.TopExtensionStats) > 10 {
		prescan.TopExtensionStats = prescan.TopExtensionStats[:10]
	}
	for _, dep := range dependencies {
		if !dep.Available {
			prescan.HasRelevantIssues = true
			break
		}
	}

	sort.Slice(files, func(i, j int) bool {
		return strings.ToLower(files[i].RelPath) < strings.ToLower(files[j].RelPath)
	})

	emitPrescanProgress(
		"complete",
		fmt.Sprintf("Prescan complete: %d non-email file(s) inventoried and %d date records cached", len(files), dateMetadataCached),
		"",
	)

	return prescan, files, nil
}

func newInventorySnapshot(sourceDir string, prescan *PrescanResult, files []classifiedFile) *InventorySnapshot {
	return &InventorySnapshot{
		SourceDir: sourceDir,
		CreatedAt: time.Now(),
		prescan:   clonePrescanResult(prescan),
		files:     append([]classifiedFile(nil), files...),
	}
}

func (snapshot *InventorySnapshot) PrescanResult() *PrescanResult {
	if snapshot == nil {
		return nil
	}
	return clonePrescanResult(snapshot.prescan)
}

func (snapshot *InventorySnapshot) MatchesSource(sourceDir string) bool {
	if snapshot == nil {
		return false
	}
	current, err := filepath.Abs(sourceDir)
	if err != nil {
		current = sourceDir
	}
	cached, err := filepath.Abs(snapshot.SourceDir)
	if err != nil {
		cached = snapshot.SourceDir
	}
	return current == cached
}

func clonePrescanResult(prescan *PrescanResult) *PrescanResult {
	if prescan == nil {
		return nil
	}
	clone := *prescan
	clone.FlaggedDirs = append([]string(nil), prescan.FlaggedDirs...)
	clone.Dependencies = append([]DependencyStatus(nil), prescan.Dependencies...)
	clone.ExtensionStats = append([]ExtensionStat(nil), prescan.ExtensionStats...)
	clone.TopExtensionStats = append([]ExtensionStat(nil), prescan.TopExtensionStats...)
	return &clone
}

func classify(file sourceFile) classifiedFile {
	classified := classifiedFile{
		sourceFile:        file,
		ContentSearchable: false,
		SearchMethod:      MethodFilenameOnly,
		SearchabilityNote: "filename search only",
	}

	switch {
	case directTextExtensions[file.Extension]:
		classified.ContentSearchable = true
		classified.SearchMethod = MethodDirectText
		classified.SearchabilityNote = "direct text search"
	case openXMLExtensions[file.Extension]:
		classified.ContentSearchable = true
		classified.SearchMethod = MethodOpenXML
		classified.SearchabilityNote = "OpenXML text extraction"
	case file.Extension == ".zip":
		classified.ContentSearchable = true
		classified.SearchMethod = MethodZipArchive
		classified.SearchabilityNote = "ZIP archive member search"
	case file.Extension == ".pdf":
		if HasPDFToText() {
			classified.ContentSearchable = true
			classified.SearchMethod = MethodPDFToText
			classified.SearchabilityNote = "PDF text extraction via pdftotext"
		} else {
			classified.SearchabilityNote = "pdftotext not installed; filename search only"
		}
	case sofficeExtensions[file.Extension]:
		if HasSoffice() {
			classified.ContentSearchable = true
			classified.SearchMethod = MethodSofficeText
			classified.SearchabilityNote = "text extraction via soffice"
		} else {
			classified.SearchabilityNote = "soffice not installed; filename search only"
		}
	}

	return classified
}

func extractSearchText(file classifiedFile) (string, error) {
	switch file.SearchMethod {
	case MethodDirectText:
		return readDirectText(file.Path)
	case MethodOpenXML:
		return readOpenXML(file.Path, file.Extension)
	case MethodPDFToText:
		return readPDFText(file.Path)
	case MethodSofficeText:
		return readSofficeText(file.Path)
	default:
		return "", nil
	}
}

func readDirectText(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.ToLower(string(data)), nil
}

func readOpenXML(path, ext string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	return readOpenXMLReader(&zr.Reader, ext)
}

func readOpenXMLBytes(data []byte, ext string) (string, error) {
	reader := bytes.NewReader(data)
	zr, err := zip.NewReader(reader, int64(len(data)))
	if err != nil {
		return "", err
	}
	return readOpenXMLReader(zr, ext)
}

func readOpenXMLReader(zr *zip.Reader, ext string) (string, error) {
	var wanted func(string) bool
	switch ext {
	case ".docx":
		wanted = func(name string) bool { return strings.HasPrefix(name, "word/") && strings.HasSuffix(name, ".xml") }
	case ".xlsx":
		wanted = func(name string) bool {
			return name == "xl/sharedStrings.xml" || (strings.HasPrefix(name, "xl/worksheets/") && strings.HasSuffix(name, ".xml"))
		}
	case ".pptx":
		wanted = func(name string) bool {
			return strings.HasPrefix(name, "ppt/slides/") && strings.HasSuffix(name, ".xml")
		}
	default:
		return "", nil
	}

	var buf strings.Builder
	for _, file := range zr.File {
		if !wanted(file.Name) {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		buf.WriteString(extractXMLText(data))
		buf.WriteByte('\n')
	}
	return strings.ToLower(buf.String()), nil
}

func extractXMLText(data []byte) string {
	unescaped := html.UnescapeString(string(data))
	unescaped = strings.ReplaceAll(unescaped, "</w:p>", " ")
	unescaped = strings.ReplaceAll(unescaped, "</a:p>", " ")
	unescaped = strings.ReplaceAll(unescaped, "</row>", " ")
	unescaped = xmlTagPattern.ReplaceAllString(unescaped, " ")
	unescaped = spacePattern.ReplaceAllString(unescaped, " ")
	return strings.TrimSpace(unescaped)
}

func readPDFText(path string) (string, error) {
	pdftotext, ok := dependencyPath("pdftotext")
	if !ok {
		return "", fmt.Errorf("pdftotext not found")
	}
	cmd := exec.Command(pdftotext, "-layout", path, "-")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s", msg)
	}
	return strings.ToLower(stdout.String()), nil
}

func readSofficeText(path string) (string, error) {
	soffice, ok := dependencyPath("soffice")
	if !ok {
		return "", fmt.Errorf("soffice not found")
	}
	tmpDir, err := os.MkdirTemp("", "file-hunter-soffice-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	cmd := exec.Command(soffice, "--headless", "--convert-to", "txt:Text", "--outdir", tmpDir, path)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s", msg)
	}

	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) + ".txt"
	txtPath := filepath.Join(tmpDir, base)
	if _, err := os.Stat(txtPath); err == nil {
		return readDirectText(txtPath)
	}

	matches, globErr := filepath.Glob(filepath.Join(tmpDir, "*.txt"))
	if globErr == nil && len(matches) == 1 {
		return readDirectText(matches[0])
	}
	if globErr == nil && len(matches) > 1 {
		return "", fmt.Errorf("soffice produced multiple text outputs; unable to choose the correct one")
	}
	return "", fmt.Errorf("soffice conversion completed but did not produce a text file")
}

func scanZipArchive(file classifiedFile, terms []string, pathSearchEnabled, contentSearchEnabled bool, maxContentBytes int64, maxZipBytes int64) ([]archiveMemberMatch, []string) {
	zr, err := zip.OpenReader(file.Path)
	if err != nil {
		return nil, []string{fmt.Sprintf("ZIP archive could not be opened: %v", err)}
	}
	defer zr.Close()

	var matches []archiveMemberMatch
	var warnings []string
	var unsafeCount, encryptedCount, nestedCount, tooLargeCount, bombCount, unreadableCount int
	var totalUncompressed int64

	for index, member := range zr.File {
		if index >= maxZipMembers {
			warnings = append(warnings, fmt.Sprintf("ZIP member limit reached after %d entries; remaining entries were not scanned", maxZipMembers))
			break
		}
		if member.FileInfo().IsDir() {
			continue
		}
		internalPath, ok := safeArchiveInternalPath(member.Name)
		if !ok {
			unsafeCount++
			continue
		}
		baseName := pathpkg.Base(internalPath)
		if scanner.ShouldSkipFileName(baseName) {
			continue
		}
		memberSize := int64(member.UncompressedSize64)
		totalUncompressed += memberSize
		if totalUncompressed > maxZipBytes {
			warnings = append(warnings, fmt.Sprintf("ZIP uncompressed byte limit reached after %d bytes; remaining entries were not scanned", maxZipBytes))
			break
		}

		extension := strings.ToLower(pathpkg.Ext(internalPath))
		filenameHits := []string{}
		filenameHitCounts := map[string]int{}
		filenameHitTotal := 0
		if pathSearchEnabled {
			filenameHits, filenameHitCounts, filenameHitTotal = findHits(strings.ToLower(baseName), terms)
		}

		contentHits := []string{}
		contentHitCounts := map[string]int{}
		contentHitTotal := 0
		contentStatus := "archive_member_filename_only"
		archiveStatus := "scanned"
		note := "ZIP member filename searched"

		switch {
		case nestedArchiveExtensions[extension]:
			nestedCount++
			archiveStatus = "nested_archive_skipped"
			note = "nested archive recorded by filename only"
		case member.Flags&zipEncryptedFlag != 0:
			encryptedCount++
			archiveStatus = "encrypted_or_unreadable"
			note = "encrypted ZIP member; content not searched"
		case isZipBombRisk(member):
			bombCount++
			archiveStatus = "skipped_safety_limit"
			note = "ZIP member content skipped due to suspicious compression ratio"
		case contentSearchEnabled:
			method, searchable, searchNote := classifyArchiveMember(extension)
			if searchable {
				limit := maxZipMemberContentBytes
				if maxContentBytes > 0 && maxContentBytes < limit {
					limit = maxContentBytes
				}
				if memberSize > limit {
					tooLargeCount++
					archiveStatus = "skipped_safety_limit"
					note = fmt.Sprintf("ZIP member content skipped; uncompressed size %d exceeds limit %d", memberSize, limit)
				} else {
					text, readErr := readArchiveMemberText(member, method, extension)
					if readErr != nil {
						unreadableCount++
						archiveStatus = "encrypted_or_unreadable"
						note = fmt.Sprintf("ZIP member content could not be read: %v", readErr)
					} else {
						contentStatus = "archive_member_searched"
						note = "ZIP member " + searchNote
						contentHits, contentHitCounts, contentHitTotal = findHits(text, terms)
					}
				}
			} else {
				note = "ZIP member is not a supported content-searchable type"
			}
		}

		if len(filenameHits) == 0 && len(contentHits) == 0 {
			continue
		}
		matches = append(matches, archiveMemberMatch{
			InternalPath:      internalPath,
			BaseName:          baseName,
			Extension:         extension,
			SizeBytes:         memberSize,
			FilenameHits:      filenameHits,
			FilenameHitCounts: filenameHitCounts,
			FilenameHitTotal:  filenameHitTotal,
			ContentHits:       contentHits,
			ContentHitCounts:  contentHitCounts,
			ContentHitTotal:   contentHitTotal,
			ContentStatus:     contentStatus,
			ArchiveStatus:     archiveStatus,
			Note:              note,
		})
	}

	if unsafeCount > 0 {
		warnings = append(warnings, fmt.Sprintf("skipped %d ZIP member(s) with unsafe internal paths", unsafeCount))
	}
	if encryptedCount > 0 {
		warnings = append(warnings, fmt.Sprintf("skipped content for %d encrypted ZIP member(s)", encryptedCount))
	}
	if nestedCount > 0 {
		warnings = append(warnings, fmt.Sprintf("recorded %d nested archive member(s) by filename only", nestedCount))
	}
	if tooLargeCount > 0 {
		warnings = append(warnings, fmt.Sprintf("skipped content for %d ZIP member(s) over the per-member safety limit", tooLargeCount))
	}
	if bombCount > 0 {
		warnings = append(warnings, fmt.Sprintf("skipped content for %d ZIP member(s) with suspicious compression ratios", bombCount))
	}
	if unreadableCount > 0 {
		warnings = append(warnings, fmt.Sprintf("skipped content for %d unreadable ZIP member(s)", unreadableCount))
	}
	return matches, warnings
}

func safeArchiveInternalPath(name string) (string, bool) {
	normalized := strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	if normalized == "" || strings.HasPrefix(normalized, "/") {
		return "", false
	}
	if len(normalized) >= 2 && normalized[1] == ':' {
		return "", false
	}
	cleaned := pathpkg.Clean(normalized)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	return cleaned, true
}

func isZipBombRisk(member *zip.File) bool {
	if member.CompressedSize64 == 0 {
		return member.UncompressedSize64 >= minZipCompressionBombBytes
	}
	return member.UncompressedSize64 >= minZipCompressionBombBytes &&
		member.UncompressedSize64/member.CompressedSize64 >= maxZipCompressionBombRatio
}

func classifyArchiveMember(ext string) (FileSearchMethod, bool, string) {
	switch {
	case directTextExtensions[ext]:
		return MethodDirectText, true, "direct text searched"
	case openXMLExtensions[ext]:
		return MethodOpenXML, true, "OpenXML text searched"
	case ext == ".pdf" && HasPDFToText():
		return MethodPDFToText, true, "PDF text searched via pdftotext"
	case sofficeExtensions[ext] && HasSoffice():
		return MethodSofficeText, true, "text searched via soffice"
	default:
		return MethodFilenameOnly, false, ""
	}
}

func readArchiveMemberText(member *zip.File, method FileSearchMethod, ext string) (string, error) {
	data, err := readZipMemberBytes(member)
	if err != nil {
		return "", err
	}
	switch method {
	case MethodDirectText:
		return strings.ToLower(string(data)), nil
	case MethodOpenXML:
		return readOpenXMLBytes(data, ext)
	case MethodPDFToText, MethodSofficeText:
		tmpDir, err := os.MkdirTemp("", "file-hunter-zip-member-*")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(tmpDir)
		tmpPath := filepath.Join(tmpDir, "member"+ext)
		if err := os.WriteFile(tmpPath, data, 0600); err != nil {
			return "", err
		}
		if method == MethodPDFToText {
			return readPDFText(tmpPath)
		}
		return readSofficeText(tmpPath)
	default:
		return "", nil
	}
}

func readZipMemberBytes(member *zip.File) ([]byte, error) {
	rc, err := member.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func MaxZipBytesFromGB(gb int) int64 {
	if gb < MinMaxZipGB {
		gb = MinMaxZipGB
	}
	if gb > MaxMaxZipGB {
		gb = MaxMaxZipGB
	}
	return int64(gb) * 1024 * 1024 * 1024
}

func NormalizeMaxZipBytes(value int64) int64 {
	if value <= 0 {
		return MaxZipBytesFromGB(DefaultMaxZipGB)
	}
	minBytes := MaxZipBytesFromGB(MinMaxZipGB)
	if value < minBytes {
		return minBytes
	}
	maxBytes := MaxZipBytesFromGB(MaxMaxZipGB)
	if value > maxBytes {
		return maxBytes
	}
	return value
}

func findHits(haystack string, terms []string) ([]string, map[string]int, int) {
	if haystack == "" {
		return nil, map[string]int{}, 0
	}
	matches := make([]string, 0, len(terms))
	counts := make(map[string]int)
	total := 0
	for _, term := range terms {
		count := countOccurrences(haystack, strings.ToLower(term))
		if count == 0 {
			continue
		}
		matches = append(matches, term)
		counts[term] = count
		total += count
	}
	return matches, counts, total
}

type folderFallbackStats struct {
	relPath              string
	sizeBytes            int64
	hasContentSearchable bool
	hasFilenameHit       bool
	flagged              bool
}

func findFolderFallbackMatches(sourceDir string, files []classifiedFile, terms []string, filenameMatchedDirs map[string]bool) []folderFallbackMatch {
	statsByDir := map[string]*folderFallbackStats{}
	for _, file := range files {
		dir := filepath.ToSlash(filepath.Dir(file.RelPath))
		if dir == "." || dir == "/" || dir == "" {
			continue
		}
		for current := dir; current != "." && current != "/" && current != ""; current = filepath.ToSlash(filepath.Dir(current)) {
			stats := statsByDir[current]
			if stats == nil {
				stats = &folderFallbackStats{relPath: current}
				statsByDir[current] = stats
			}
			stats.sizeBytes += file.SizeBytes
			stats.flagged = stats.flagged || file.Flagged
			if file.ContentSearchable {
				stats.hasContentSearchable = true
			}
			if filenameMatchedDirs[current] {
				stats.hasFilenameHit = true
			}
			parent := filepath.ToSlash(filepath.Dir(current))
			if parent == current {
				break
			}
		}
	}

	matches := make([]folderFallbackMatch, 0)
	for relDir, stats := range statsByDir {
		if stats.hasContentSearchable || stats.hasFilenameHit {
			continue
		}
		baseName := filepath.Base(filepath.FromSlash(relDir))
		hits, counts, total := findHits(strings.ToLower(baseName), terms)
		if len(hits) == 0 {
			continue
		}
		matches = append(matches, folderFallbackMatch{
			Path:      filepath.Join(sourceDir, filepath.FromSlash(relDir)),
			RelPath:   relDir,
			BaseName:  baseName,
			SizeBytes: stats.sizeBytes,
			Hits:      hits,
			HitCounts: counts,
			HitTotal:  total,
			Flagged:   stats.flagged,
		})
	}

	sort.Slice(matches, func(i, j int) bool {
		return strings.ToLower(matches[i].RelPath) < strings.ToLower(matches[j].RelPath)
	})
	return matches
}

func maxMatchesReached(summary *RunSummary, maxMatches int) bool {
	return maxMatches > 0 && summary.MatchedFiles >= maxMatches
}

func hitCategoryTotals(summary *RunSummary) (int, int, int) {
	return sumHits(summary.FilenameHitsByKeyword), sumHits(summary.FolderHitsByKeyword), sumHits(summary.ContentHitsByKeyword)
}

func sumHits(values map[string]int) int {
	total := 0
	for _, count := range values {
		total += count
	}
	return total
}

func keywordHitStats(summary *RunSummary) []KeywordHitStat {
	stats := make([]KeywordHitStat, 0, len(summary.Terms))
	seen := map[string]bool{}
	for _, term := range summary.Terms {
		if seen[term] {
			continue
		}
		seen[term] = true
		stat := KeywordHitStat{
			Keyword:      term,
			FilenameHits: summary.FilenameHitsByKeyword[term],
			FolderHits:   summary.FolderHitsByKeyword[term],
			ContentHits:  summary.ContentHitsByKeyword[term],
		}
		stat.TotalHits = stat.FilenameHits + stat.FolderHits + stat.ContentHits
		if stat.TotalHits > 0 {
			stats = append(stats, stat)
		}
	}
	sort.SliceStable(stats, func(i, j int) bool {
		if stats[i].TotalHits != stats[j].TotalHits {
			return stats[i].TotalHits > stats[j].TotalHits
		}
		return strings.ToLower(stats[i].Keyword) < strings.ToLower(stats[j].Keyword)
	})
	return stats
}

func markAncestorDirs(marked map[string]bool, sourceDir, dir string) {
	for {
		rel := filepath.ToSlash(scanner.RelPath(sourceDir, dir))
		if rel == "." || rel == "/" || rel == "" {
			return
		}
		marked[rel] = true
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}

func countOccurrences(haystack, needle string) int {
	if needle == "" {
		return 0
	}
	count := 0
	for offset := 0; ; {
		idx := strings.Index(haystack[offset:], needle)
		if idx == -1 {
			return count
		}
		count++
		offset += idx + len(needle)
		if offset >= len(haystack) {
			return count
		}
	}
}

func hasFlaggedAncestor(path string, flagged map[string]bool) bool {
	for current := filepath.Dir(path); current != "" && current != string(filepath.Separator) && current != "."; current = filepath.Dir(current) {
		if flagged[current] {
			return true
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return false
}

func displayExt(ext string) string {
	if ext == "" {
		return "[no_ext]"
	}
	return ext
}

func contentStatus(file classifiedFile, note string) string {
	if file.ContentSearchable && note == file.SearchabilityNote {
		return "searched"
	}
	if file.ContentSearchable && strings.Contains(strings.ToLower(note), "failed") {
		return "fallback_filename_only"
	}
	return "filename_only"
}

func sourceRelativeOutputPath(root, relPath string) (string, error) {
	cleaned := filepath.Clean(filepath.FromSlash(relPath))
	if cleaned == "." || cleaned == "" {
		return "", fmt.Errorf("empty relative path")
	}
	if filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe relative path %q", relPath)
	}
	return filepath.Join(root, cleaned), nil
}

func copySourceFileOnce(copied map[string]string, sourcePath, relPath, outputDir, copyDir string) (string, bool, error) {
	if copiedPath, ok := copied[relPath]; ok {
		return copiedPath, false, nil
	}
	destPath, err := sourceRelativeOutputPath(copyDir, relPath)
	if err != nil {
		return "", false, err
	}
	if err := copyFile(sourcePath, destPath); err != nil {
		return "", false, err
	}
	copiedPath := scanner.RelPath(outputDir, destPath)
	copied[relPath] = copiedPath
	return copiedPath, true, nil
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func copyDirTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if path != src && info.IsDir() && scanner.ShouldSkipWalkDir(path) {
			return filepath.SkipDir
		}
		if scanner.ShouldSkipFileName(info.Name()) {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		return copyFile(path, target)
	})
}

func writeInventory(path string, rows []InventoryRow) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write([]string{
		"base_name",
		"source_directory_path",
		"extension",
		"size_bytes",
		"size_human",
		"content_searchable",
		"search_method",
		"searchability_note",
		"flagged_parent",
	}); err != nil {
		return err
	}

	for _, row := range rows {
		if err := w.Write([]string{
			row.BaseName,
			directoryOnlyPath(row.SourceRelativePath),
			row.Extension,
			fmt.Sprintf("%d", row.SizeBytes),
			humanSize(row.SizeBytes),
			fmt.Sprintf("%t", row.ContentSearchable),
			row.SearchMethod,
			row.SearchabilityNote,
			fmt.Sprintf("%t", row.FlaggedParent),
		}); err != nil {
			return err
		}
	}
	return w.Error()
}

func writeManifest(path string, rows []MatchRow) error {
	return writeCSV(path, manifestHeaders(), technicalManifestRecords(rows))
}

func writeReviewManifest(path string, rows []MatchRow) error {
	return writeCSV(path, reviewManifestHeaders(), reviewManifestRecords(rows))
}

func writeManifestWorkbook(path string, rows []MatchRow) error {
	return writeWorkbook(path, "Technical Manifest", manifestHeaders(), technicalManifestRecords(rows), "TechnicalManifest")
}

func writeReviewWorkbook(path string, rows []MatchRow) error {
	return writeWorkbook(path, "Reviewer Manifest", reviewManifestHeaders(), reviewManifestRecords(rows), "ReviewerManifest")
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

func manifestHeaders() []string {
	return []string{
		"base_name",
		"source_directory_path",
		"extension",
		"filename_hit_keywords",
		"filename_hit_counts",
		"filename_hit_total",
		"folder_hit_keywords",
		"folder_hit_counts",
		"folder_hit_total",
		"content_hit_keywords",
		"content_hit_counts",
		"content_hit_total",
		"archive_path",
		"archive_internal_path",
		"archive_status",
		"hit_summary",
		"document_date",
		"document_date_source",
		"date_status",
		"date_note",
		"filesystem_created",
		"filesystem_modified",
		"embedded_created",
		"embedded_modified",
		"content_status",
		"copied_directory_path",
		"size_bytes",
		"size_human",
		"note",
	}
}

func technicalManifestRecords(rows []MatchRow) [][]string {
	records := make([][]string, 0, len(rows))
	for _, row := range rows {
		records = append(records, []string{
			row.BaseName,
			sourceDirectoryPath(row),
			row.Extension,
			strings.Join(row.FilenameHits, " | "),
			formatKeywordCounts(row.FilenameHits, row.FilenameHitCounts),
			fmt.Sprintf("%d", row.FilenameHitTotal),
			strings.Join(row.FolderHits, " | "),
			formatKeywordCounts(row.FolderHits, row.FolderHitCounts),
			fmt.Sprintf("%d", row.FolderHitTotal),
			strings.Join(row.ContentHits, " | "),
			formatKeywordCounts(row.ContentHits, row.ContentHitCounts),
			fmt.Sprintf("%d", row.ContentHitTotal),
			row.ArchivePath,
			row.ArchiveInternalPath,
			row.ArchiveStatus,
			formatHitSummary(row),
			row.DocumentDate,
			row.DocumentDateSource,
			row.DateStatus,
			row.DateNote,
			row.FilesystemCreated,
			row.FilesystemModified,
			row.EmbeddedCreated,
			row.EmbeddedModified,
			row.ContentStatus,
			directoryOnlyPath(row.CopiedFilePath),
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
		"source_directory_path",
		"copied_directory_path",
		"extension",
		"hit_summary",
		"filename_hit_total",
		"folder_hit_total",
		"content_hit_total",
		"document_date",
		"document_date_source",
		"date_status",
		"content_status",
		"archive_path",
		"archive_internal_path",
		"archive_status",
		"size_bytes",
		"size_human",
		"note",
	}
}

func reviewManifestRecords(rows []MatchRow) [][]string {
	reviewRows := append([]MatchRow(nil), rows...)
	sort.SliceStable(reviewRows, func(i, j int) bool {
		left, right := reviewRows[i], reviewRows[j]
		switch {
		case left.ContentHitTotal != right.ContentHitTotal:
			return left.ContentHitTotal > right.ContentHitTotal
		case left.FilenameHitTotal != right.FilenameHitTotal:
			return left.FilenameHitTotal > right.FilenameHitTotal
		case left.FolderHitTotal != right.FolderHitTotal:
			return left.FolderHitTotal > right.FolderHitTotal
		default:
			return strings.ToLower(left.SourceRelativePath) < strings.ToLower(right.SourceRelativePath)
		}
	})

	records := make([][]string, 0, len(reviewRows))
	for _, row := range reviewRows {
		records = append(records, []string{
			row.BaseName,
			sourceDirectoryPath(row),
			directoryOnlyPath(row.CopiedFilePath),
			row.Extension,
			formatHitSummary(row),
			fmt.Sprintf("%d", row.FilenameHitTotal),
			fmt.Sprintf("%d", row.FolderHitTotal),
			fmt.Sprintf("%d", row.ContentHitTotal),
			row.DocumentDate,
			reviewDateSourceLabel(row.DocumentDateSource),
			reviewDateStatusLabel(row.DateStatus),
			reviewContentStatusLabel(row.ContentStatus),
			row.ArchivePath,
			row.ArchiveInternalPath,
			reviewArchiveStatusLabel(row.ArchiveStatus),
			fmt.Sprintf("%d", row.SizeBytes),
			humanSize(row.SizeBytes),
			reviewNote(row),
		})
	}
	return records
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
	case "filename_hit_total", "folder_hit_total", "content_hit_total", "size_bytes":
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

func sourceDirectoryPath(row MatchRow) string {
	if row.ArchivePath != "" {
		return directoryOnlyPath(row.ArchivePath)
	}
	return directoryOnlyPath(row.SourceRelativePath)
}

func directoryOnlyPath(rel string) string {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if rel == "" || rel == "." {
		return ""
	}
	dir := pathpkg.Dir(rel)
	if dir == "." {
		return ""
	}
	return dir
}

func humanSize(sizeBytes int64) string {
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

func formatHitSummary(row MatchRow) string {
	parts := make([]string, 0, len(row.FilenameHits)+len(row.FolderHits)+len(row.ContentHits))
	for _, term := range row.FilenameHits {
		parts = append(parts, fmt.Sprintf("filename:%s=%d", term, row.FilenameHitCounts[term]))
	}
	for _, term := range row.FolderHits {
		parts = append(parts, fmt.Sprintf("folder:%s=%d", term, row.FolderHitCounts[term]))
	}
	for _, term := range row.ContentHits {
		parts = append(parts, fmt.Sprintf("content:%s=%d", term, row.ContentHitCounts[term]))
	}
	return strings.Join(parts, " ; ")
}

func reviewContentStatusLabel(status string) string {
	switch status {
	case "searched":
		return "content searched"
	case "filename_only":
		return "filename match only"
	case "archive_member_filename_only":
		return "archive member filename match"
	case "folder_fallback":
		return "folder-name fallback"
	case "archive_container":
		return "archive filename match"
	case "":
		return ""
	default:
		return strings.ReplaceAll(status, "_", " ")
	}
}

func reviewDateSourceLabel(source string) string {
	switch source {
	case "filesystem_created":
		return "file created time"
	case "embedded_created":
		return "embedded document created time"
	case "filesystem_modified":
		return "file modified time"
	case "embedded_modified":
		return "embedded document modified time"
	case "folder_year":
		return "folder year fallback"
	case "":
		return ""
	default:
		return strings.ReplaceAll(source, "_", " ")
	}
}

func reviewDateStatusLabel(status string) string {
	switch status {
	case string(DateStatusInRange):
		return "in range"
	case string(DateStatusUnknown):
		return "unknown"
	case string(DateStatusExcludedPreRange):
		return "excluded pre-range"
	case "":
		return ""
	default:
		return strings.ReplaceAll(status, "_", " ")
	}
}

func reviewArchiveStatusLabel(status string) string {
	switch status {
	case "scanned":
		return "archive scanned"
	case "container_filename_match":
		return "archive filename match"
	case "nested_archive_skipped":
		return "nested archive skipped"
	case "encrypted_or_unreadable":
		return "encrypted or unreadable"
	case "skipped_safety_limit":
		return "skipped by safety limit"
	case "":
		return ""
	default:
		return strings.ReplaceAll(status, "_", " ")
	}
}

func reviewNote(row MatchRow) string {
	parts := []string{}

	switch {
	case row.ArchiveInternalPath != "" && row.ContentHitTotal > 0 && row.FilenameHitTotal > 0:
		parts = append(parts, "Archive member matched in filename and content.")
	case row.ArchiveInternalPath != "" && row.ContentHitTotal > 0:
		parts = append(parts, "Archive member matched in content.")
	case row.ArchiveInternalPath != "" && row.FilenameHitTotal > 0:
		parts = append(parts, "Archive member matched by filename.")
	case row.FolderHitTotal > 0:
		parts = append(parts, "Folder name matched.")
	case row.ContentHitTotal > 0 && row.FilenameHitTotal > 0:
		parts = append(parts, "Matched in filename and content.")
	case row.ContentHitTotal > 0:
		parts = append(parts, "Matched in content.")
	case row.FilenameHitTotal > 0:
		parts = append(parts, "Matched by filename.")
	}

	switch row.ContentStatus {
	case "filename_only":
		parts = append(parts, "Content was not searched for this file type.")
	case "archive_member_filename_only":
		parts = append(parts, "Member content was not searched for this file type.")
	case "folder_fallback":
		parts = append(parts, "Used folder-name fallback because the folder had no content-searchable files and no file-name matches.")
	}

	if label := reviewDateSourceLabel(row.DocumentDateSource); label != "" {
		parts = append(parts, fmt.Sprintf("Date used: %s.", label))
	}

	if row.ArchiveStatus != "" && row.ArchiveStatus != "scanned" {
		parts = append(parts, fmt.Sprintf("Archive status: %s.", reviewArchiveStatusLabel(row.ArchiveStatus)))
	}

	return strings.Join(parts, " ")
}

func writeConfig(path string, summary *RunSummary, cfg Config) error {
	payload := map[string]any{
		"run_timestamp":              summary.RunTimestamp,
		"run_dir":                    summary.RunDir,
		"manifest_path":              filepath.Base(summary.ManifestPath),
		"review_manifest_path":       filepath.Base(summary.ReviewManifestPath),
		"manifest_workbook_path":     filepath.Base(summary.ManifestWorkbookPath),
		"review_workbook_path":       filepath.Base(summary.ReviewWorkbookPath),
		"mode":                       summary.Mode,
		"search_scope":               summary.SearchScope,
		"max_matches":                summary.MaxMatches,
		"max_content_bytes":          summary.MaxContentBytes,
		"max_zip_bytes":              summary.MaxZipBytes,
		"used_inventory_snapshot":    summary.UsedInventorySnapshot,
		"stopped_by_max_matches":     summary.StoppedByMaxMatches,
		"source_root_label":          summary.SourceRootLabel,
		"output_root_label":          summary.OutputRootLabel,
		"terms":                      summary.Terms,
		"rejected_keywords":          summary.RejectedKeywords,
		"dependencies":               summary.Dependencies,
		"files_discovered":           summary.FilesDiscovered,
		"ignored_email_files":        summary.IgnoredEmailFiles,
		"content_searchable_files":   summary.ContentSearchableFiles,
		"filename_only_files":        summary.FilenameOnlyFiles,
		"scan_bytes":                 summary.ScanBytes,
		"start_date":                 summary.StartDate,
		"end_date":                   summary.EndDate,
		"has_date_filter":            summary.HasDateFilter,
		"date_policy":                summary.DatePolicy,
		"content_size_skipped_files": summary.ContentSizeSkippedFiles,
		"in_range_matched_files":     summary.InRangeMatchedFiles,
		"unknown_date_matched_files": summary.UnknownDateMatchedFiles,
		"excluded_pre_range_files":   summary.ExcludedPreRangeFiles,
		"dry_run":                    cfg.DryRun,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func writeReport(path string, summary *RunSummary) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := func(s string) { fmt.Fprintln(f, s) }
	w("# File Keyword Hunter Run Report")
	w("")
	w(fmt.Sprintf("Run timestamp: `%s`", summary.RunTimestamp))
	w(fmt.Sprintf("Mode: `%s`", summary.Mode))
	w(fmt.Sprintf("Search scope: `%s`", summary.SearchScope))
	if summary.MaxMatches > 0 {
		w(fmt.Sprintf("Max matched items: `%d`", summary.MaxMatches))
	}
	if summary.MaxContentBytes > 0 {
		w(fmt.Sprintf("Max content extraction bytes: `%d`", summary.MaxContentBytes))
	}
	w(fmt.Sprintf("Max ZIP uncompressed bytes: `%d`", summary.MaxZipBytes))
	w(fmt.Sprintf("Prescan snapshot used: `%t`", summary.UsedInventorySnapshot))
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
	w(fmt.Sprintf("- Non-email files discovered: %d", summary.FilesDiscovered-summary.IgnoredEmailFiles))
	w(fmt.Sprintf("- Email files ignored: %d", summary.IgnoredEmailFiles))
	w(fmt.Sprintf("- Content-searchable files: %d", summary.ContentSearchableFiles))
	w(fmt.Sprintf("- Filename-only files: %d", summary.FilenameOnlyFiles))
	w(fmt.Sprintf("- Matched files: %d", summary.MatchedFiles))
	w(fmt.Sprintf("- Copied files: %d", summary.CopiedFiles))
	w(fmt.Sprintf("- Content extraction skipped for size limit: %d", summary.ContentSizeSkippedFiles))
	if summary.StoppedByMaxMatches {
		w(fmt.Sprintf("- Stopped early after reaching max matched items: %d", summary.MaxMatches))
	}
	w(fmt.Sprintf("- In-range matched files: %d", summary.InRangeMatchedFiles))
	w(fmt.Sprintf("- Unknown-date matched files: %d", summary.UnknownDateMatchedFiles))
	w(fmt.Sprintf("- Excluded pre-range files: %d", summary.ExcludedPreRangeFiles))
	w("")

	if summary.HasDateFilter {
		w("## Date Policy")
		w("")
		w(fmt.Sprintf("- Review range: start `%s`, end `%s`", emptyIfBlank(summary.StartDate), emptyIfBlank(summary.EndDate)))
		w(fmt.Sprintf("- %s", summary.DatePolicy))
		w("- `unknown` does not necessarily mean no metadata existed.")
		w("- `unknown` may also mean the available date was not reliable for exclusion or was later than the review range.")
		w("")
	}

	w("## Byte Totals")
	w("")
	w(fmt.Sprintf("- Total corpus bytes: %d", summary.TotalBytes))
	w(fmt.Sprintf("- Non-email scan bytes: %d", summary.ScanBytes))
	w(fmt.Sprintf("- Ignored email bytes: %d", summary.IgnoredEmailBytes))
	w(fmt.Sprintf("- Content-searchable bytes: %d", summary.ContentSearchableBytes))
	w(fmt.Sprintf("- Filename-only bytes: %d", summary.FilenameOnlyBytes))
	w("")

	w("## Keyword Summary")
	w("")
	for _, term := range summary.Terms {
		w(fmt.Sprintf("- `%s`: filename occurrences `%d`, folder fallback occurrences `%d`, content occurrences `%d`", term, summary.FilenameHitsByKeyword[term], summary.FolderHitsByKeyword[term], summary.ContentHitsByKeyword[term]))
	}
	w("")

	if len(summary.ManifestRows) > 0 {
		w("## Match Artifacts")
		w("")
		w(fmt.Sprintf("- Reviewer spreadsheet: `%s`", filepath.Base(summary.ReviewWorkbookPath)))
		w(fmt.Sprintf("- Reviewer CSV: `%s`", filepath.Base(summary.ReviewManifestPath)))
		w(fmt.Sprintf("- Technical spreadsheet: `%s`", filepath.Base(summary.ManifestWorkbookPath)))
		w(fmt.Sprintf("- Technical CSV: `%s`", filepath.Base(summary.ManifestPath)))
		w("- The per-document hit list lives in the spreadsheet outputs rather than this report.")
		w("")
	}

	w("## Dependencies")
	w("")
	for _, dep := range summary.Dependencies {
		status := "available"
		if !dep.Available {
			status = "missing"
		}
		w(fmt.Sprintf("- `%s`: %s", dep.Name, status))
		if !dep.Available {
			w(fmt.Sprintf("  - %s", dep.Reason))
			w(fmt.Sprintf("  - Install: `%s`", dep.InstallHint))
		}
	}
	w("")

	if len(summary.FlaggedDirs) > 0 {
		w("## Flagged Folders")
		w("")
		for _, dir := range summary.FlaggedDirs {
			w(fmt.Sprintf("- `%s`", dir))
		}
		w("")
	}

	if len(summary.Warnings) > 0 {
		w("## Warnings")
		w("")
		for _, warning := range summary.Warnings {
			w(fmt.Sprintf("- %s", warning))
		}
		w("")
	}

	if len(summary.Errors) > 0 {
		w("## Errors")
		w("")
		for _, item := range summary.Errors {
			w(fmt.Sprintf("- %s", item))
		}
	}
	return nil
}

func formatKeywordCounts(terms []string, counts map[string]int) string {
	if len(terms) == 0 {
		return ""
	}
	parts := make([]string, 0, len(terms))
	for _, term := range terms {
		parts = append(parts, fmt.Sprintf("%s=%d", term, counts[term]))
	}
	return strings.Join(parts, " | ")
}

func emptyIfBlank(value string) string {
	if strings.TrimSpace(value) == "" {
		return "none"
	}
	return value
}
