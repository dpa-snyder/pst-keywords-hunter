package main

import (
	"context"
	"fmt"
	"keyword-hunter/filescanner"
	"sync"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx context.Context

	runMu           sync.Mutex
	running         bool
	prescanMu       sync.Mutex
	prescanCancel   context.CancelFunc
	prescanSeq      int
	prescanSnapshot *filescanner.InventorySnapshot
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) GetAppState() AppState {
	return AppState{
		Version:        "filehog-wails-v1",
		SupportedTypes: []string{"PDF", "DOCX", "XLSX", "PPTX", "Text-like", "Filename-only fallback"},
		Dependencies:   dependencyInfos(),
		Environment:    environmentInfos("", ""),
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
	if sourceDir == "" {
		return PrescanResult{}, fmt.Errorf("source directory is required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	prescanEvents := make(chan filescanner.PrescanProgress, 32)
	a.prescanMu.Lock()
	if a.prescanCancel != nil {
		a.prescanCancel()
	}
	a.prescanSeq++
	prescanSeq := a.prescanSeq
	a.prescanCancel = cancel
	a.prescanMu.Unlock()

	defer func() {
		a.prescanMu.Lock()
		if a.prescanSeq == prescanSeq {
			a.prescanCancel = nil
		}
		a.prescanMu.Unlock()
		cancel()
	}()

	done := make(chan struct{})
	go func() {
		for event := range prescanEvents {
			wailsruntime.EventsEmit(a.ctx, "prescan:progress", prescanProgressFromScanner(event))
		}
		close(done)
	}()

	snapshot, err := filescanner.BuildInventorySnapshotWithContextAndEvents(ctx, sourceDir, prescanEvents)
	close(prescanEvents)
	<-done
	if err != nil {
		return PrescanResult{}, err
	}
	a.prescanMu.Lock()
	if a.prescanSeq == prescanSeq {
		a.prescanSnapshot = snapshot
	}
	a.prescanMu.Unlock()

	converted := prescanResultFromScanner(snapshot.PrescanResult())
	if converted == nil {
		return PrescanResult{}, fmt.Errorf("failed to build prescan summary")
	}
	return *converted, nil
}

func (a *App) CancelPrescan() {
	a.prescanMu.Lock()
	defer a.prescanMu.Unlock()
	if a.prescanCancel != nil {
		a.prescanCancel()
		a.prescanCancel = nil
	}
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

	deps := filescanner.DependencyStatuses()
	for _, dep := range deps {
		if dep.Key == request.Dependency {
			result.ManualCommand = dep.InstallHint
			break
		}
	}

	if err := filescanner.InstallDependency(request.Dependency); err != nil {
		result.Reason = err.Error()
		result.Message = fmt.Sprintf("%s install failed", request.Dependency)
		return result, nil
	}
	a.clearPrescanSnapshot()

	result.Success = true
	result.Message = fmt.Sprintf("%s installed successfully", request.Dependency)
	return result, nil
}

func (a *App) clearPrescanSnapshot() {
	a.prescanMu.Lock()
	defer a.prescanMu.Unlock()
	a.prescanSnapshot = nil
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

	snapshot := a.snapshotForSource(prepared.Config.SourceDir)
	go a.executeRun(prepared.Config, snapshot)

	return RunStarted{
		Started: true,
		Mode:    map[bool]string{true: "estimate", false: "scan"}[prepared.Config.DryRun],
	}, nil
}

func (a *App) OpenArtifact(baseDir, relativePath string) error {
	target := joinArtifactPath(baseDir, relativePath)
	return openPath(target)
}

func (a *App) snapshotForSource(sourceDir string) *filescanner.InventorySnapshot {
	a.prescanMu.Lock()
	defer a.prescanMu.Unlock()
	if a.prescanSnapshot != nil && a.prescanSnapshot.MatchesSource(sourceDir) {
		return a.prescanSnapshot
	}
	return nil
}

func (a *App) executeRun(cfg filescanner.Config, snapshot *filescanner.InventorySnapshot) {
	defer func() {
		a.runMu.Lock()
		a.running = false
		a.runMu.Unlock()
	}()

	events := make(chan filescanner.Event, 100)
	done := make(chan struct{})
	progress := RunProgressPayload{}

	go func() {
		for event := range events {
			switch event.Type {
			case filescanner.EventFileStart:
				progress.CurrentFile = event.SourceFile
				progress.SearchMethod = event.SearchMethod
				progress.Searchability = event.Searchability
			case filescanner.EventMatch:
				progress.MatchedFiles = event.MatchedFiles
				progress.CopiedFiles = event.CopiedFiles
				progress.FilenameHits = event.FilenameHits
				progress.FolderHits = event.FolderHits
				progress.ContentHits = event.ContentHits
				progress.KeywordHitStats = keywordHitStatsFromScanner(event.KeywordHitStats)
				progress.OutputPath = event.OutputPath
			case filescanner.EventComplete:
				progress.TotalFiles = event.TotalFiles
				progress.MatchedFiles = event.MatchedFiles
				progress.CopiedFiles = event.CopiedFiles
				progress.FilenameHits = event.FilenameHits
				progress.FolderHits = event.FolderHits
				progress.ContentHits = event.ContentHits
				progress.KeywordHitStats = keywordHitStatsFromScanner(event.KeywordHitStats)
			}
			if event.UsedInventorySnapshot {
				progress.UsedInventorySnapshot = true
			}
			progress.EventType = fmt.Sprintf("%d", event.Type)
			progress.Message = event.Message
			progress.FileNum = event.FileNum
			if event.TotalFiles > 0 {
				progress.TotalFiles = event.TotalFiles
			}
			progress.Note = event.Note
			wailsruntime.EventsEmit(a.ctx, "run:progress", progress)
		}
		close(done)
	}()

	summary, err := filescanner.RunWithSnapshot(cfg, events, snapshot)
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
		InventoryPath:        summary.InventoryPath,
		ConfigPath:           summary.ConfigPath,
		LogPath:              summary.LogPath,
		FilesScanned:         summary.FilesScanned,
		MatchedFiles:         summary.MatchedFiles,
		CopiedFiles:          summary.CopiedFiles,
		Warnings:             summary.Warnings,
		Errors:               summary.Errors,
		OutputRoot:           cfg.OutputDir,
	}
	wailsruntime.EventsEmit(a.ctx, "run:completed", payload)
}
