package main

import (
	"context"
	"fmt"
	"io"
	"keyword-hunter/filescanner"
	"keyword-hunter/scanner"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type preparedRun struct {
	Config     filescanner.Config
	Validation ValidationResult
}

func splitKeywords(value string) []string {
	return scanner.ParseInlineKeywords(value)
}

func dependencyInfos() []DependencyInfo {
	deps := filescanner.DependencyStatuses()
	result := make([]DependencyInfo, 0, len(deps))
	for _, dep := range deps {
		result = append(result, dependencyInfoFrom(dep))
	}
	return result
}

func environmentInfos(sourceDir, outputDir string) []EnvironmentInfo {
	items := []EnvironmentInfo{
		{Key: "source_root", Name: "Source root readable", Checked: sourceDir != ""},
		{Key: "output_root", Name: "Output root writable", Checked: outputDir != ""},
		{Key: "temp_space", Name: "Temporary workspace", Checked: true},
		{Key: "artifact_open", Name: "Open reports and folders", Checked: true},
	}

	for i := range items {
		switch items[i].Key {
		case "source_root":
			if !items[i].Checked {
				items[i].Status = "Not set"
				items[i].Detail = "Choose a source root to verify read access."
				continue
			}
			if info, err := os.Stat(sourceDir); err != nil {
				items[i].Status = "Blocked"
				items[i].Detail = err.Error()
			} else if !info.IsDir() {
				items[i].Status = "Blocked"
				items[i].Detail = "Source path is not a directory."
			} else if err := canReadDir(sourceDir); err != nil {
				items[i].Status = "Blocked"
				items[i].Detail = err.Error()
			} else {
				items[i].OK = true
				items[i].Status = "Ready"
				items[i].Detail = "The source root is readable."
			}
		case "output_root":
			if !items[i].Checked {
				items[i].Status = "Not set"
				items[i].Detail = "Choose an output root to verify write access."
				continue
			}
			if err := canWriteOutputRoot(outputDir); err != nil {
				items[i].Status = "Blocked"
				items[i].Detail = err.Error()
			} else {
				items[i].OK = true
				items[i].Status = "Ready"
				items[i].Detail = "The output root is writable."
			}
		case "temp_space":
			if err := canWriteTemp(); err != nil {
				items[i].Status = "Blocked"
				items[i].Detail = err.Error()
			} else {
				items[i].OK = true
				items[i].Status = "Ready"
				items[i].Detail = "Temporary conversion space is writable."
			}
		case "artifact_open":
			if err := hasArtifactOpener(); err != nil {
				items[i].Status = "Blocked"
				items[i].Detail = err.Error()
			} else {
				items[i].OK = true
				items[i].Status = "Ready"
				items[i].Detail = "The system can open run folders and reports."
			}
		}
	}

	return items
}

func canReadDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Readdirnames(1)
	if err != nil && err != io.EOF {
		return err
	}
	return nil
}

func canWriteOutputRoot(path string) error {
	target := path
	if _, err := os.Stat(path); os.IsNotExist(err) {
		target = filepath.Dir(path)
	}
	if target == "" {
		target = "."
	}
	testFile, err := os.CreateTemp(target, ".file-hunter-write-test-*")
	if err != nil {
		return err
	}
	name := testFile.Name()
	if err := testFile.Close(); err != nil {
		return err
	}
	return os.Remove(name)
}

func canWriteTemp() error {
	dir, err := os.MkdirTemp("", "file-hunter-env-*")
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

func hasArtifactOpener() error {
	var bin string
	switch runtime.GOOS {
	case "darwin":
		bin = "open"
	case "windows":
		bin = "explorer"
	default:
		bin = "xdg-open"
	}
	_, err := exec.LookPath(bin)
	if err != nil {
		return fmt.Errorf("%s is not available", bin)
	}
	return nil
}

func resolveRunInput(input RunConfigInput) (*preparedRun, error) {
	result := &preparedRun{
		Validation: ValidationResult{
			Ready:            true,
			Errors:           []string{},
			Warnings:         []string{},
			MergedTerms:      []string{},
			RejectedKeywords: []scanner.RejectedKeyword{},
			Conflicts:        []scanner.ConflictGroup{},
			Dependencies:     []DependencyInfo{},
			Environment:      []EnvironmentInfo{},
		},
	}

	if input.SourceDir == "" {
		result.Validation.Errors = append(result.Validation.Errors, "Source directory is required.")
	}
	if input.OutputDir == "" {
		result.Validation.Errors = append(result.Validation.Errors, "Output directory is required.")
	}
	if input.SourceDir != "" && input.OutputDir != "" {
		if err := scanner.ValidateRoots(input.SourceDir, input.OutputDir); err != nil {
			result.Validation.Errors = append(result.Validation.Errors, err.Error())
		}
	}
	searchScopeValue := strings.ToLower(strings.TrimSpace(input.SearchScope))
	searchScope := filescanner.NormalizeSearchScope(filescanner.SearchScope(searchScopeValue))
	if searchScopeValue != "" && string(searchScope) != searchScopeValue {
		result.Validation.Errors = append(result.Validation.Errors, "Search scope must be both, paths, or content.")
	}
	if input.MaxMatches < 0 {
		result.Validation.Errors = append(result.Validation.Errors, "Max matched items must be 0 or greater.")
	}
	if input.MaxContentMB < 0 {
		result.Validation.Errors = append(result.Validation.Errors, "Max content extraction size must be 0 or greater.")
	}
	if input.MaxZipGB == 0 {
		input.MaxZipGB = filescanner.DefaultMaxZipGB
	}
	if input.MaxZipGB < filescanner.MinMaxZipGB || input.MaxZipGB > filescanner.MaxMaxZipGB {
		result.Validation.Errors = append(result.Validation.Errors, fmt.Sprintf("ZIP safety limit must be between %d and %d GiB.", filescanner.MinMaxZipGB, filescanner.MaxMaxZipGB))
	}
	startDate, err := scanner.ParseDateInput(input.StartDate)
	if err != nil {
		result.Validation.Errors = append(result.Validation.Errors, fmt.Sprintf("Start date must use YYYY-MM-DD format: %v", err))
	}
	endDate, err := scanner.ParseDateInput(input.EndDate)
	if err != nil {
		result.Validation.Errors = append(result.Validation.Errors, fmt.Sprintf("End date must use YYYY-MM-DD format: %v", err))
	}
	if startDate != nil && endDate != nil && startDate.After(*endDate) {
		result.Validation.Errors = append(result.Validation.Errors, "Start date must not be after end date.")
	}

	var fileTerms []string
	if input.KeywordsFile != "" {
		loaded, err := scanner.LoadKeywordsFile(input.KeywordsFile)
		if err != nil {
			result.Validation.Errors = append(result.Validation.Errors, fmt.Sprintf("Failed to load keywords file: %v", err))
		} else {
			fileTerms = loaded
		}
	}

	terms := scanner.MergeKeywordLists(splitKeywords(input.KeywordsText), fileTerms)
	result.Validation.MergedTerms = append(result.Validation.MergedTerms, terms...)
	if len(terms) == 0 && !input.EstimateMode {
		result.Validation.Errors = append(result.Validation.Errors, "At least one keyword is required for a scan.")
	}

	conflicts := scanner.FindKeywordConflicts(terms)
	resolvedTerms := append([]string(nil), terms...)
	rejected := []scanner.RejectedKeyword{}
	if len(conflicts) > 0 {
		for _, conflict := range conflicts {
			choice := input.ConflictSelections[conflict.Normalized]
			if choice == "" {
				result.Validation.Conflicts = append(result.Validation.Conflicts, conflict)
				continue
			}
			var resolveErr error
			resolvedTerms, rejected, resolveErr = applyConflictChoice(resolvedTerms, rejected, conflict, choice)
			if resolveErr != nil {
				result.Validation.Errors = append(result.Validation.Errors, resolveErr.Error())
			}
		}
		if len(result.Validation.Conflicts) > 0 {
			result.Validation.Ready = false
		}
	}
	result.Validation.MergedTerms = resolvedTerms
	result.Validation.RejectedKeywords = rejected
	for _, dep := range dependencyInfos() {
		result.Validation.Dependencies = append(result.Validation.Dependencies, dep)
		if !dep.Available {
			result.Validation.Warnings = append(result.Validation.Warnings, fmt.Sprintf("%s is unavailable. Affected formats will fall back to filename-only search.", dep.Name))
		}
	}
	result.Validation.Environment = environmentInfos(input.SourceDir, input.OutputDir)

	if len(result.Validation.Errors) > 0 {
		result.Validation.Ready = false
	}

	result.Config = filescanner.Config{
		SourceDir:        input.SourceDir,
		OutputDir:        input.OutputDir,
		Terms:            resolvedTerms,
		RejectedKeywords: rejected,
		StartDate:        startDate,
		EndDate:          endDate,
		DryRun:           input.EstimateMode,
		SearchScope:      searchScope,
		MaxMatches:       input.MaxMatches,
		MaxContentBytes:  int64(input.MaxContentMB) * 1024 * 1024,
		MaxZipBytes:      filescanner.MaxZipBytesFromGB(input.MaxZipGB),
	}

	return result, nil
}

func applyConflictChoice(terms []string, rejected []scanner.RejectedKeyword, conflict scanner.ConflictGroup, choice string) ([]string, []scanner.RejectedKeyword, error) {
	resolved, newRejected, err := scanner.ResolveKeywordConflict(terms, conflict, choice)
	if err != nil {
		return nil, nil, err
	}
	rejected = append(rejected, newRejected...)
	return resolved, rejected, nil
}

func prescanResultFromScanner(prescan *filescanner.PrescanResult) *PrescanResult {
	if prescan == nil {
		return nil
	}
	result := &PrescanResult{
		SourceDir:              prescan.SourceDir,
		FilesDiscovered:        prescan.FilesDiscovered,
		IgnoredEmailFiles:      prescan.IgnoredEmailFiles,
		ContentSearchableFiles: prescan.ContentSearchableFiles,
		FilenameOnlyFiles:      prescan.FilenameOnlyFiles,
		TotalBytes:             prescan.TotalBytes,
		ScanBytes:              prescan.ScanBytes,
		IgnoredEmailBytes:      prescan.IgnoredEmailBytes,
		ContentSearchableBytes: prescan.ContentSearchableBytes,
		FilenameOnlyBytes:      prescan.FilenameOnlyBytes,
		FlaggedDirs:            append([]string(nil), prescan.FlaggedDirs...),
		Dependencies:           []DependencyInfo{},
		ExtensionStats:         []ExtensionStat{},
		TopExtensionStats:      []ExtensionStat{},
		HasRelevantIssues:      prescan.HasRelevantIssues,
	}
	for _, dep := range prescan.Dependencies {
		result.Dependencies = append(result.Dependencies, dependencyInfoFrom(dep))
	}
	for _, stat := range prescan.ExtensionStats {
		result.ExtensionStats = append(result.ExtensionStats, extensionStatFrom(stat))
	}
	for _, stat := range prescan.TopExtensionStats {
		result.TopExtensionStats = append(result.TopExtensionStats, extensionStatFrom(stat))
	}
	return result
}

func prescanProgressFromScanner(progress filescanner.PrescanProgress) PrescanProgressPayload {
	return PrescanProgressPayload{
		Stage:                  progress.Stage,
		Message:                progress.Message,
		CurrentFile:            progress.CurrentFile,
		FilesDiscovered:        progress.FilesDiscovered,
		IgnoredEmailFiles:      progress.IgnoredEmailFiles,
		ContentSearchableFiles: progress.ContentSearchableFiles,
		FilenameOnlyFiles:      progress.FilenameOnlyFiles,
		TotalBytes:             progress.TotalBytes,
		ScanBytes:              progress.ScanBytes,
		DateMetadataCached:     progress.DateMetadataCached,
	}
}

func openPath(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("explorer", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	return cmd.Start()
}

func joinArtifactPath(baseDir, relativePath string) string {
	if relativePath == "" {
		return baseDir
	}
	if filepath.IsAbs(relativePath) {
		return relativePath
	}
	return filepath.Join(baseDir, filepath.FromSlash(relativePath))
}

func chooseDirectory(ctx context.Context, prompt string) (string, error) {
	if runtime.GOOS == "darwin" {
		return chooseDirectoryAppleScript(prompt)
	}
	return wailsruntime.OpenDirectoryDialog(ctx, wailsruntime.OpenDialogOptions{
		Title:                prompt,
		CanCreateDirectories: true,
	})
}

func chooseFile(ctx context.Context, prompt string, filters []wailsruntime.FileFilter) (string, error) {
	if runtime.GOOS == "darwin" {
		return chooseFileAppleScript(prompt)
	}
	return wailsruntime.OpenFileDialog(ctx, wailsruntime.OpenDialogOptions{
		Title:   prompt,
		Filters: filters,
	})
}

func chooseDirectoryAppleScript(prompt string) (string, error) {
	script := fmt.Sprintf(`try
POSIX path of (choose folder with prompt %q)
on error number -128
return ""
end try`, prompt)
	out, err := exec.Command("osascript", "-e", script).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func chooseFileAppleScript(prompt string) (string, error) {
	script := fmt.Sprintf(`try
POSIX path of (choose file with prompt %q)
on error number -128
return ""
end try`, prompt)
	out, err := exec.Command("osascript", "-e", script).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
