package main

import (
	"keyword-hunter/filescanner"
	"keyword-hunter/scanner"
)

type DependencyInfo struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Available   bool   `json:"available"`
	InstallHint string `json:"installHint"`
	Reason      string `json:"reason"`
	AutoInstall bool   `json:"autoInstall"`
}

type EnvironmentInfo struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Status  string `json:"status"`
	Detail  string `json:"detail"`
	Checked bool   `json:"checked"`
}

type ExtensionStat struct {
	Extension string `json:"extension"`
	Count     int    `json:"count"`
	SizeBytes int64  `json:"sizeBytes"`
}

type AppState struct {
	Version        string            `json:"version"`
	SupportedTypes []string          `json:"supportedTypes"`
	Dependencies   []DependencyInfo  `json:"dependencies"`
	Environment    []EnvironmentInfo `json:"environment"`
}

type RunConfigInput struct {
	SourceDir          string            `json:"sourceDir"`
	OutputDir          string            `json:"outputDir"`
	KeywordsText       string            `json:"keywordsText"`
	KeywordsFile       string            `json:"keywordsFile"`
	StartDate          string            `json:"startDate"`
	EndDate            string            `json:"endDate"`
	EstimateMode       bool              `json:"estimateMode"`
	SearchScope        string            `json:"searchScope"`
	MaxMatches         int               `json:"maxMatches"`
	MaxContentMB       int               `json:"maxContentMB"`
	MaxZipGB           int               `json:"maxZipGB"`
	ConflictSelections map[string]string `json:"conflictSelections"`
}

type PrescanResult struct {
	SourceDir              string           `json:"sourceDir"`
	FilesDiscovered        int              `json:"filesDiscovered"`
	IgnoredEmailFiles      int              `json:"ignoredEmailFiles"`
	ContentSearchableFiles int              `json:"contentSearchableFiles"`
	FilenameOnlyFiles      int              `json:"filenameOnlyFiles"`
	TotalBytes             int64            `json:"totalBytes"`
	ScanBytes              int64            `json:"scanBytes"`
	IgnoredEmailBytes      int64            `json:"ignoredEmailBytes"`
	ContentSearchableBytes int64            `json:"contentSearchableBytes"`
	FilenameOnlyBytes      int64            `json:"filenameOnlyBytes"`
	FlaggedDirs            []string         `json:"flaggedDirs"`
	Dependencies           []DependencyInfo `json:"dependencies"`
	ExtensionStats         []ExtensionStat  `json:"extensionStats"`
	TopExtensionStats      []ExtensionStat  `json:"topExtensionStats"`
	HasRelevantIssues      bool             `json:"hasRelevantIssues"`
}

type PrescanProgressPayload struct {
	Stage                  string `json:"stage"`
	Message                string `json:"message"`
	CurrentFile            string `json:"currentFile"`
	FilesDiscovered        int    `json:"filesDiscovered"`
	IgnoredEmailFiles      int    `json:"ignoredEmailFiles"`
	ContentSearchableFiles int    `json:"contentSearchableFiles"`
	FilenameOnlyFiles      int    `json:"filenameOnlyFiles"`
	TotalBytes             int64  `json:"totalBytes"`
	ScanBytes              int64  `json:"scanBytes"`
	DateMetadataCached     int    `json:"dateMetadataCached"`
}

type ValidationResult struct {
	Ready            bool                      `json:"ready"`
	Errors           []string                  `json:"errors"`
	Warnings         []string                  `json:"warnings"`
	MergedTerms      []string                  `json:"mergedTerms"`
	RejectedKeywords []scanner.RejectedKeyword `json:"rejectedKeywords"`
	Conflicts        []scanner.ConflictGroup   `json:"conflicts"`
	Dependencies     []DependencyInfo          `json:"dependencies"`
	Environment      []EnvironmentInfo         `json:"environment"`
	Prescan          *PrescanResult            `json:"prescan"`
}

type InstallRequest struct {
	Dependency string `json:"dependency"`
}

type InstallResult struct {
	Dependency    string `json:"dependency"`
	Success       bool   `json:"success"`
	Message       string `json:"message"`
	ManualCommand string `json:"manualCommand"`
	Reason        string `json:"reason"`
}

type RunStarted struct {
	Started bool   `json:"started"`
	Mode    string `json:"mode"`
}

type RunProgressPayload struct {
	EventType             string           `json:"eventType"`
	Message               string           `json:"message"`
	CurrentFile           string           `json:"currentFile"`
	FileNum               int              `json:"fileNum"`
	TotalFiles            int              `json:"totalFiles"`
	MatchedFiles          int              `json:"matchedFiles"`
	CopiedFiles           int              `json:"copiedFiles"`
	FilenameHits          int              `json:"filenameHits"`
	FolderHits            int              `json:"folderHits"`
	ContentHits           int              `json:"contentHits"`
	KeywordHitStats       []KeywordHitStat `json:"keywordHitStats"`
	UsedInventorySnapshot bool             `json:"usedInventorySnapshot"`
	OutputPath            string           `json:"outputPath"`
	SearchMethod          string           `json:"searchMethod"`
	Searchability         string           `json:"searchability"`
	Note                  string           `json:"note"`
}

type KeywordHitStat struct {
	Keyword      string `json:"keyword"`
	FilenameHits int    `json:"filenameHits"`
	FolderHits   int    `json:"folderHits"`
	ContentHits  int    `json:"contentHits"`
	TotalHits    int    `json:"totalHits"`
}

type RunCompletedPayload struct {
	RunTimestamp         string   `json:"runTimestamp"`
	RunDir               string   `json:"runDir"`
	ReportPath           string   `json:"reportPath"`
	ManifestPath         string   `json:"manifestPath"`
	ReviewManifestPath   string   `json:"reviewManifestPath"`
	ManifestWorkbookPath string   `json:"manifestWorkbookPath"`
	ReviewWorkbookPath   string   `json:"reviewWorkbookPath"`
	InventoryPath        string   `json:"inventoryPath"`
	ConfigPath           string   `json:"configPath"`
	LogPath              string   `json:"logPath"`
	FilesScanned         int      `json:"filesScanned"`
	MatchedFiles         int      `json:"matchedFiles"`
	CopiedFiles          int      `json:"copiedFiles"`
	Warnings             []string `json:"warnings"`
	Errors               []string `json:"errors"`
	OutputRoot           string   `json:"outputRoot"`
}

func dependencyInfoFrom(dep filescanner.DependencyStatus) DependencyInfo {
	return DependencyInfo{
		Key:         dep.Key,
		Name:        dep.Name,
		Available:   dep.Available,
		InstallHint: dep.InstallHint,
		Reason:      dep.Reason,
		AutoInstall: dep.AutoInstall,
	}
}

func extensionStatFrom(stat filescanner.ExtensionStat) ExtensionStat {
	return ExtensionStat{
		Extension: stat.Extension,
		Count:     stat.Count,
		SizeBytes: stat.SizeBytes,
	}
}

func keywordHitStatsFromScanner(stats []filescanner.KeywordHitStat) []KeywordHitStat {
	result := make([]KeywordHitStat, 0, len(stats))
	for _, stat := range stats {
		result = append(result, KeywordHitStat{
			Keyword:      stat.Keyword,
			FilenameHits: stat.FilenameHits,
			FolderHits:   stat.FolderHits,
			ContentHits:  stat.ContentHits,
			TotalHits:    stat.TotalHits,
		})
	}
	return result
}
