package filescanner

import (
	"keyword-hunter/scanner"
	"time"
)

type EventType int

const (
	EventDiscovery EventType = iota
	EventFileStart
	EventMatch
	EventFileDone
	EventError
	EventComplete
)

type Event struct {
	Type                  EventType
	SourceFile            string
	Message               string
	FileNum               int
	TotalFiles            int
	MatchedFiles          int
	CopiedFiles           int
	FilenameHits          int
	FolderHits            int
	ContentHits           int
	KeywordHitStats       []KeywordHitStat
	UsedInventorySnapshot bool
	OutputPath            string
	SearchMethod          string
	Searchability         string
	Note                  string
}

type Config struct {
	SourceDir        string
	OutputDir        string
	Terms            []string
	RejectedKeywords []scanner.RejectedKeyword
	StartDate        *time.Time
	EndDate          *time.Time
	DryRun           bool
	SearchScope      SearchScope
	MaxMatches       int
	MaxContentBytes  int64
	MaxZipBytes      int64
}

type SearchScope string

const (
	SearchScopeBoth    SearchScope = "both"
	SearchScopePaths   SearchScope = "paths"
	SearchScopeContent SearchScope = "content"
)

func NormalizeSearchScope(scope SearchScope) SearchScope {
	switch scope {
	case SearchScopePaths, SearchScopeContent, SearchScopeBoth:
		return scope
	default:
		return SearchScopeBoth
	}
}

func (scope SearchScope) AllowsPathSearch() bool {
	return NormalizeSearchScope(scope) != SearchScopeContent
}

func (scope SearchScope) AllowsContentSearch() bool {
	return NormalizeSearchScope(scope) != SearchScopePaths
}

type InventorySnapshot struct {
	SourceDir string
	CreatedAt time.Time
	prescan   *PrescanResult
	files     []classifiedFile
}

type PrescanProgress struct {
	Stage                  string
	Message                string
	CurrentFile            string
	FilesDiscovered        int
	IgnoredEmailFiles      int
	ContentSearchableFiles int
	FilenameOnlyFiles      int
	TotalBytes             int64
	ScanBytes              int64
	DateMetadataCached     int
}

type DependencyStatus struct {
	Key         string
	Name        string
	Available   bool
	InstallHint string
	Reason      string
	AutoInstall bool
}

type FileSearchMethod string

const (
	MethodFilenameOnly FileSearchMethod = "filename_only"
	MethodDirectText   FileSearchMethod = "direct_text"
	MethodOpenXML      FileSearchMethod = "openxml"
	MethodPDFToText    FileSearchMethod = "pdftotext"
	MethodSofficeText  FileSearchMethod = "soffice_text"
	MethodZipArchive   FileSearchMethod = "zip_archive"
)

type InventoryRow struct {
	SourceRelativePath string
	BaseName           string
	Extension          string
	SizeBytes          int64
	ContentSearchable  bool
	SearchMethod       string
	SearchabilityNote  string
	FlaggedParent      bool
}

type MatchRow struct {
	SourceRelativePath  string
	BaseName            string
	Extension           string
	FilenameHits        []string
	FilenameHitCounts   map[string]int
	FilenameHitTotal    int
	FolderHits          []string
	FolderHitCounts     map[string]int
	FolderHitTotal      int
	ContentHits         []string
	ContentHitCounts    map[string]int
	ContentHitTotal     int
	ArchivePath         string
	ArchiveInternalPath string
	ArchiveStatus       string
	DocumentDate        string
	DocumentDateSource  string
	DateStatus          string
	DateNote            string
	FilesystemCreated   string
	FilesystemModified  string
	EmbeddedCreated     string
	EmbeddedModified    string
	ContentStatus       string
	CopiedFilePath      string
	Note                string
	SizeBytes           int64
}

type ExtensionStat struct {
	Extension string
	Count     int
	SizeBytes int64
}

type KeywordHitStat struct {
	Keyword      string
	FilenameHits int
	FolderHits   int
	ContentHits  int
	TotalHits    int
}

type RunSummary struct {
	RunTimestamp            string
	RunDir                  string
	ReportPath              string
	ManifestPath            string
	ReviewManifestPath      string
	ManifestWorkbookPath    string
	ReviewWorkbookPath      string
	InventoryPath           string
	ConfigPath              string
	LogPath                 string
	Mode                    string
	SearchScope             SearchScope
	MaxMatches              int
	MaxContentBytes         int64
	MaxZipBytes             int64
	UsedInventorySnapshot   bool
	StoppedByMaxMatches     bool
	StartDate               string
	EndDate                 string
	HasDateFilter           bool
	DatePolicy              string
	SourceRootLabel         string
	OutputRootLabel         string
	Terms                   []string
	RejectedKeywords        []scanner.RejectedKeyword
	Dependencies            []DependencyStatus
	FilesDiscovered         int
	FilesScanned            int
	IgnoredEmailFiles       int
	MatchedFiles            int
	CopiedFiles             int
	TotalBytes              int64
	ScanBytes               int64
	IgnoredEmailBytes       int64
	ContentSearchableFiles  int
	ContentSearchableBytes  int64
	FilenameOnlyFiles       int
	FilenameOnlyBytes       int64
	FilenameHitsByKeyword   map[string]int
	FolderHitsByKeyword     map[string]int
	ContentHitsByKeyword    map[string]int
	ContentSizeSkippedFiles int
	InRangeMatchedFiles     int
	UnknownDateMatchedFiles int
	ExcludedPreRangeFiles   int
	Warnings                []string
	Errors                  []string
	FlaggedDirs             []string
	ExtensionStats          []ExtensionStat
	InventoryRows           []InventoryRow
	ManifestRows            []MatchRow
}

type PrescanResult struct {
	SourceDir              string
	FilesDiscovered        int
	IgnoredEmailFiles      int
	ContentSearchableFiles int
	FilenameOnlyFiles      int
	TotalBytes             int64
	ScanBytes              int64
	IgnoredEmailBytes      int64
	ContentSearchableBytes int64
	FilenameOnlyBytes      int64
	FlaggedDirs            []string
	Dependencies           []DependencyStatus
	ExtensionStats         []ExtensionStat
	TopExtensionStats      []ExtensionStat
	HasRelevantIssues      bool
}
