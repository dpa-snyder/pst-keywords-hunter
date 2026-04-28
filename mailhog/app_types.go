package main

import "keyword-hunter/scanner"

type DependencyInfo struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Available   bool   `json:"available"`
	InstallHint string `json:"installHint"`
	Reason      string `json:"reason"`
	Required    bool   `json:"required"`
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

type AppState struct {
	Version        string            `json:"version"`
	SupportedTypes []string          `json:"supportedTypes"`
	Dependencies   []DependencyInfo  `json:"dependencies"`
	Environment    []EnvironmentInfo `json:"environment"`
	CLICommand     string            `json:"cliCommand"`
}

type RunConfigInput struct {
	SourceDir          string            `json:"sourceDir"`
	OutputDir          string            `json:"outputDir"`
	KeywordsText       string            `json:"keywordsText"`
	KeywordsFile       string            `json:"keywordsFile"`
	EnableDateFilter   bool              `json:"enableDateFilter"`
	StartDate          string            `json:"startDate"`
	EndDate            string            `json:"endDate"`
	EstimateMode       bool              `json:"estimateMode"`
	EnablePST          bool              `json:"enablePST"`
	EnableOST          bool              `json:"enableOST"`
	EnableEML          bool              `json:"enableEML"`
	EnableMSG          bool              `json:"enableMSG"`
	EnableMBOX         bool              `json:"enableMBOX"`
	ConflictSelections map[string]string `json:"conflictSelections"`
}

type PrescanResult struct {
	SourceDir         string           `json:"sourceDir"`
	Counts            map[string]int   `json:"counts"`
	FlaggedDirs       []string         `json:"flaggedDirs"`
	Dependencies      []DependencyInfo `json:"dependencies"`
	PrecheckedTypes   []string         `json:"precheckedTypes"`
	HasRelevantIssues bool             `json:"hasRelevantIssues"`
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
	Counts           map[string]int            `json:"counts"`
	FlaggedDirs      []string                  `json:"flaggedDirs"`
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
	EventType        string `json:"eventType"`
	Message          string `json:"message"`
	CurrentFile      string `json:"currentFile"`
	CurrentKeyword   string `json:"currentKeyword"`
	FileNum          int    `json:"fileNum"`
	TotalFiles       int    `json:"totalFiles"`
	TotalHits        int    `json:"totalHits"`
	UnknownDateCount int    `json:"unknownDateCount"`
	SkippedCount     int    `json:"skippedCount"`
	OutputDir        string `json:"outputDir"`
	Note             string `json:"note"`
}

type RunCompletedPayload struct {
	RunTimestamp         string   `json:"runTimestamp"`
	RunDir               string   `json:"runDir"`
	ReportPath           string   `json:"reportPath"`
	ManifestPath         string   `json:"manifestPath"`
	ReviewManifestPath   string   `json:"reviewManifestPath"`
	ManifestWorkbookPath string   `json:"manifestWorkbookPath"`
	ReviewWorkbookPath   string   `json:"reviewWorkbookPath"`
	ConfigPath           string   `json:"configPath"`
	LogPath              string   `json:"logPath"`
	TotalFiles           int      `json:"totalFiles"`
	TotalHits            int      `json:"totalHits"`
	UnknownDateCount     int      `json:"unknownDateCount"`
	SkippedCount         int      `json:"skippedCount"`
	Warnings             []string `json:"warnings"`
	Errors               []string `json:"errors"`
	OutputRoot           string   `json:"outputRoot"`
}
