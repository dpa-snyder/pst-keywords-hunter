package main

import (
	"context"
	"fmt"
	"keyword-hunter/scanner"
	"sync"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx context.Context

	runMu   sync.Mutex
	running bool
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) GetAppState() AppState {
	return AppState{
		Version:        "mailhog-wails-v1",
		SupportedTypes: []string{"PST", "OST", "EML", "MSG", "MBOX"},
		Dependencies:   dependencyInfos(),
		Environment:    environmentInfos("", ""),
		CLICommand:     "go run ../cmd/keyword-hunter-cli",
	}
}

func (a *App) PickSourceDirectory() (string, error) {
	return chooseDirectory(a.ctx, "Choose Source Root")
}

func (a *App) PickOutputDirectory() (string, error) {
	return chooseDirectory(a.ctx, "Choose Output Root")
}

func (a *App) PickKeywordsFile() (string, error) {
	return chooseFile(a.ctx, "Choose Keywords File", []wailsruntime.FileFilter{
		{DisplayName: "Text Files", Pattern: "*.txt;*.csv"},
		{DisplayName: "All Files", Pattern: "*"},
	})
}

func (a *App) PrescanSource(sourceDir string) (PrescanResult, error) {
	result := PrescanResult{
		SourceDir:       sourceDir,
		Counts:          map[string]int{},
		FlaggedDirs:     []string{},
		Dependencies:    dependencyInfos(),
		PrecheckedTypes: []string{},
	}
	if sourceDir == "" {
		return result, fmt.Errorf("source directory is required")
	}

	discovered, err := scanner.DiscoverFiles(sourceDir, map[scanner.FileType]bool{
		scanner.TypePST:  true,
		scanner.TypeOST:  true,
		scanner.TypeEML:  true,
		scanner.TypeMSG:  true,
		scanner.TypeMBOX: true,
	})
	if err != nil {
		return result, err
	}

	counts := scanner.CountFiles(discovered)
	for _, ft := range scanner.AllFileTypes() {
		result.Counts[ft.String()] = counts[ft]
		if counts[ft] > 0 {
			result.PrecheckedTypes = append(result.PrecheckedTypes, ft.String())
		}
	}
	flagged, err := scanner.ScanFlaggedFolders(sourceDir)
	if err == nil {
		for _, dir := range flagged {
			result.FlaggedDirs = append(result.FlaggedDirs, scanner.RelPath(sourceDir, dir))
		}
	}

	for i, dep := range result.Dependencies {
		switch dep.Key {
		case "readpst":
			dep.Required = counts[scanner.TypePST] > 0 || counts[scanner.TypeOST] > 0
		case "python3", "extract_msg":
			dep.Required = counts[scanner.TypeMSG] > 0
		}
		result.Dependencies[i] = dep
		if dep.Required && !dep.Available {
			result.HasRelevantIssues = true
		}
	}

	return result, nil
}

func (a *App) ValidateRun(input RunConfigInput) (ValidationResult, error) {
	prepared, err := resolveRunInput(input)
	if err != nil {
		return ValidationResult{}, err
	}
	return prepared.Validation, nil
}

func (a *App) InstallDependency(request InstallRequest) (InstallResult, error) {
	result := InstallResult{
		Dependency: request.Dependency,
		Success:    false,
	}

	switch request.Dependency {
	case "readpst":
		result.ManualCommand = scanner.ReadPSTDependencyStatus().InstallHint
		if err := scanner.InstallReadPST(); err != nil {
			result.Reason = err.Error()
			result.Message = "readpst install failed"
			return result, nil
		}
		result.Success = true
		result.Message = "readpst installed successfully"
		return result, nil
	case "python3":
		result.ManualCommand = scanner.Python3DependencyStatus().InstallHint
		if err := scanner.InstallPython3(); err != nil {
			result.Reason = err.Error()
			result.Message = "python3 install failed"
			return result, nil
		}
		result.Success = true
		result.Message = "python3 installed successfully"
		return result, nil
	case "extract_msg":
		result.ManualCommand = scanner.ExtractMSGDependencyStatus().InstallHint
		if err := scanner.InstallHighFidelityMSG(); err != nil {
			result.Reason = err.Error()
			result.Message = "extract-msg install failed"
			return result, nil
		}
		result.Success = true
		result.Message = "extract-msg installed successfully"
		return result, nil
	default:
		return result, fmt.Errorf("unknown dependency: %s", request.Dependency)
	}
}

func (a *App) StartRun(input RunConfigInput) (RunStarted, error) {
	prepared, err := resolveRunInput(input)
	if err != nil {
		return RunStarted{}, err
	}
	if !prepared.Validation.Ready {
		return RunStarted{}, fmt.Errorf("run configuration is not ready")
	}

	a.runMu.Lock()
	if a.running {
		a.runMu.Unlock()
		return RunStarted{}, fmt.Errorf("a run is already in progress")
	}
	a.running = true
	a.runMu.Unlock()

	wailsruntime.EventsEmit(a.ctx, "run:started", RunStarted{
		Started: true,
		Mode:    map[bool]string{true: "estimate", false: "scan"}[prepared.Config.DryRun],
	})

	go a.executeRun(prepared.Config)

	return RunStarted{
		Started: true,
		Mode:    map[bool]string{true: "estimate", false: "scan"}[prepared.Config.DryRun],
	}, nil
}

func (a *App) OpenArtifact(baseDir, relativePath string) error {
	target := joinArtifactPath(baseDir, relativePath)
	return openPath(target)
}

func (a *App) executeRun(cfg scanner.Config) {
	defer func() {
		a.runMu.Lock()
		a.running = false
		a.runMu.Unlock()
	}()

	events := make(chan scanner.Event, 100)
	done := make(chan struct{})
	progress := RunProgressPayload{}

	go func() {
		for event := range events {
			switch event.Type {
			case scanner.EventSearching:
				progress.CurrentKeyword = event.Term
			case scanner.EventFileStart:
				progress.CurrentFile = event.SourceFile
			case scanner.EventMatch:
				progress.TotalHits++
			case scanner.EventUnknownDate:
				progress.TotalHits++
				progress.UnknownDateCount++
			case scanner.EventSkipped, scanner.EventError:
				progress.SkippedCount++
			case scanner.EventComplete:
				progress.TotalFiles = event.TotalFiles
			}
			progress.EventType = fmt.Sprintf("%d", event.Type)
			progress.Message = event.Message
			progress.FileNum = event.FileNum
			if event.TotalFiles > 0 {
				progress.TotalFiles = event.TotalFiles
			}
			progress.OutputDir = event.OutputDir
			progress.Note = event.Note
			wailsruntime.EventsEmit(a.ctx, "run:progress", progress)
		}
		close(done)
	}()

	summary, err := scanner.RunWithSummary(cfg, events)
	<-done
	if err != nil {
		wailsruntime.EventsEmit(a.ctx, "run:failed", map[string]string{"message": err.Error()})
		return
	}

	payload := RunCompletedPayload{
		RunTimestamp:         summary.RunTimestamp,
		RunDir:               summary.RunDir,
		ReportPath:           summary.ReportPath,
		ManifestPath:         summary.ManifestPath,
		ReviewManifestPath:   summary.ReviewManifestPath,
		ManifestWorkbookPath: summary.ManifestWorkbookPath,
		ReviewWorkbookPath:   summary.ReviewWorkbookPath,
		ConfigPath:           summary.ConfigPath,
		LogPath:              summary.LogPath,
		TotalFiles:           summary.FilesScanned,
		TotalHits:            sumKeywordHits(summary.KeywordHits),
		UnknownDateCount:     summary.UnknownDateTotal,
		SkippedCount:         summary.SkippedTotal,
		Warnings:             summary.Warnings,
		Errors:               summary.Errors,
		OutputRoot:           cfg.OutputDir,
	}
	wailsruntime.EventsEmit(a.ctx, "run:completed", payload)
}

func sumKeywordHits(values map[string]int) int {
	total := 0
	for _, v := range values {
		total += v
	}
	return total
}
