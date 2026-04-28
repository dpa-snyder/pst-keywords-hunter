export namespace main {
  export type DependencyInfo = {
    key: string;
    name: string;
    available: boolean;
    installHint: string;
    reason: string;
    autoInstall: boolean;
  };

  export type EnvironmentInfo = {
    key: string;
    name: string;
    ok: boolean;
    status: string;
    detail: string;
    checked: boolean;
  };

  export type ExtensionStat = {
    extension: string;
    count: number;
    sizeBytes: number;
  };

  export type AppState = {
    version: string;
    supportedTypes: string[];
    dependencies: DependencyInfo[];
    environment: EnvironmentInfo[];
  };

  export type InstallRequest = {
    dependency: string;
  };

  export type InstallResult = {
    dependency: string;
    success: boolean;
    message: string;
    manualCommand: string;
    reason: string;
  };

  export type PrescanResult = {
    sourceDir: string;
    filesDiscovered: number;
    ignoredEmailFiles: number;
    contentSearchableFiles: number;
    filenameOnlyFiles: number;
    totalBytes: number;
    scanBytes: number;
    ignoredEmailBytes: number;
    contentSearchableBytes: number;
    filenameOnlyBytes: number;
    flaggedDirs: string[];
    dependencies: DependencyInfo[];
    extensionStats: ExtensionStat[];
    topExtensionStats: ExtensionStat[];
    hasRelevantIssues: boolean;
  };

  export type PrescanProgressPayload = {
    stage: string;
    message: string;
    currentFile: string;
    filesDiscovered: number;
    ignoredEmailFiles: number;
    contentSearchableFiles: number;
    filenameOnlyFiles: number;
    totalBytes: number;
    scanBytes: number;
    dateMetadataCached: number;
  };

  export type RunConfigInput = {
    sourceDir: string;
    outputDir: string;
    keywordsText: string;
    keywordsFile: string;
    startDate: string;
    endDate: string;
    estimateMode: boolean;
    searchScope: string;
    maxMatches: number;
    maxContentMB: number;
    maxZipGB: number;
    conflictSelections: Record<string, string>;
  };

  export type RunStarted = {
    started: boolean;
    mode: string;
  };

  export type KeywordHitStat = {
    keyword: string;
    filenameHits: number;
    folderHits: number;
    contentHits: number;
    totalHits: number;
  };

  export type RunProgressPayload = {
    eventType: string;
    message: string;
    currentFile: string;
    fileNum: number;
    totalFiles: number;
    matchedFiles: number;
    copiedFiles: number;
    filenameHits: number;
    folderHits: number;
    contentHits: number;
    keywordHitStats: KeywordHitStat[];
    usedInventorySnapshot: boolean;
    outputPath: string;
    searchMethod: string;
    searchability: string;
    note: string;
  };

  export type ValidationResult = {
    ready: boolean;
    errors: string[];
    warnings: string[];
    mergedTerms: string[];
    rejectedKeywords: Array<{ requested: string; normalized: string; kept: string; reason: string }>;
    conflicts: Array<{ normalized: string; options: string[] }>;
    dependencies: DependencyInfo[];
    environment: EnvironmentInfo[];
    prescan?: PrescanResult | null;
  };
}
