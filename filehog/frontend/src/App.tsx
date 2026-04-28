import { useEffect, useMemo, useRef, useState } from "react";
import "./App.css";
import {
  CancelPrescan,
  GetAppState,
  InstallDependency,
  OpenArtifact,
  PickKeywordsFile,
  PickOutputDirectory,
  PickSourceDirectory,
  PrescanSource,
  StartRun,
  ValidateRun,
} from "../wailsjs/go/main/App";
import { EventsOff, EventsOn, Quit, WindowMinimise, WindowToggleMaximise } from "../wailsjs/runtime/runtime";

type DependencyInfo = {
  key: string;
  name: string;
  available: boolean;
  installHint: string;
  reason: string;
  autoInstall: boolean;
};

type EnvironmentInfo = {
  key: string;
  name: string;
  ok: boolean;
  status: string;
  detail: string;
  checked: boolean;
};

type ConflictGroup = {
  normalized: string;
  options: string[];
};

type RejectedKeyword = {
  requested: string;
  normalized: string;
  kept: string;
  reason: string;
};

type ExtensionStat = {
  extension: string;
  count: number;
  sizeBytes: number;
};

type ValidationResult = {
  ready: boolean;
  errors: string[];
  warnings: string[];
  mergedTerms: string[];
  rejectedKeywords: RejectedKeyword[];
  conflicts: ConflictGroup[];
  dependencies: DependencyInfo[];
  environment: EnvironmentInfo[];
  prescan?: PrescanResult | null;
};

type PrescanResult = {
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

type PrescanProgressPayload = {
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

type RunProgressPayload = {
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

type KeywordHitStat = {
  keyword: string;
  filenameHits: number;
  folderHits: number;
  contentHits: number;
  totalHits: number;
};

type RunCompletedPayload = {
  runTimestamp: string;
  runDir: string;
  reportPath: string;
  manifestPath: string;
  reviewManifestPath: string;
  manifestWorkbookPath: string;
  reviewWorkbookPath: string;
  inventoryPath: string;
  configPath: string;
  logPath: string;
  filesScanned: number;
  matchedFiles: number;
  copiedFiles: number;
  warnings: string[];
  errors: string[];
  outputRoot: string;
};

type AppState = {
  version: string;
  supportedTypes: string[];
  dependencies: DependencyInfo[];
  environment: EnvironmentInfo[];
};

type ThemeName = "rust" | "forest" | "newsprint" | "grey" | "blue";
type ThemeMode = "light" | "dark";

type RunConfigInput = {
  sourceDir: string;
  outputDir: string;
  keywordsText: string;
  keywordsFile: string;
  startDate: string;
  endDate: string;
  estimateMode: boolean;
  searchScope: "both" | "paths" | "content";
  maxMatches: number;
  maxContentMB: number;
  maxZipGB: number;
  conflictSelections: Record<string, string>;
};

const initialConfig: RunConfigInput = {
  sourceDir: "",
  outputDir: "",
  keywordsText: "",
  keywordsFile: "",
  startDate: "",
  endDate: "",
  estimateMode: false,
  searchScope: "both",
  maxMatches: 0,
  maxContentMB: 0,
  maxZipGB: 4,
  conflictSelections: {},
};

const THEME_STORAGE_KEY = "filehog-theme";
const MODE_STORAGE_KEY = "filehog-mode";

const themeOptions: Array<{ value: ThemeName; label: string; blurb: string }> = [
  { value: "rust", label: "Rust + Dust", blurb: "Warm field notes and barn-ink contrast." },
  { value: "forest", label: "Forest + Soil", blurb: "Deep greens and earthier shadows." },
  { value: "newsprint", label: "Newsprint + Red Stamp", blurb: "Archive-paper neutrals with stamped accents." },
  { value: "grey", label: "Grey + Steel", blurb: "Cool graphite surfaces with neutral contrast." },
  { value: "blue", label: "Blue + Harbor", blurb: "Maritime blues with brighter navigation cues." },
];

function readStoredTheme(): ThemeName {
  if (typeof window === "undefined") {
    return "rust";
  }
  const stored = window.localStorage.getItem(THEME_STORAGE_KEY);
  if (stored === "rust" || stored === "forest" || stored === "newsprint" || stored === "grey" || stored === "blue") {
    return stored;
  }
  return "rust";
}

function readStoredMode(): ThemeMode {
  if (typeof window === "undefined") {
    return "light";
  }
  const stored = window.localStorage.getItem(MODE_STORAGE_KEY);
  if (stored === "light" || stored === "dark") {
    return stored;
  }
  return "light";
}

function formatStatusTimestamp(date: Date): string {
  return date.toLocaleString([], {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
}

function formatBytes(value: number): string {
  if (!Number.isFinite(value)) {
    return "0 B";
  }
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let amount = value;
  let unit = 0;
  while (amount >= 1024 && unit < units.length - 1) {
    amount /= 1024;
    unit += 1;
  }
  return `${amount.toFixed(amount >= 100 || unit === 0 ? 0 : 2)} ${units[unit]}`;
}

function SunIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <circle cx="12" cy="12" r="4.25" fill="currentColor" />
      <path
        d="M12 1.75v3M12 19.25v3M1.75 12h3M19.25 12h3M4.22 4.22l2.12 2.12M17.66 17.66l2.12 2.12M17.66 6.34l2.12-2.12M4.22 19.78l2.12-2.12"
        fill="none"
        stroke="currentColor"
        strokeLinecap="round"
        strokeWidth="1.8"
      />
    </svg>
  );
}

function MoonIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <path
        d="M15.4 2.35c-1.08 1.72-1.7 3.75-1.7 5.92 0 6.17 5 11.17 11.17 11.17.26 0 .52-.01.78-.03a10.5 10.5 0 1 1-10.25-17.06Z"
        fill="currentColor"
      />
    </svg>
  );
}

function MinimiseIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <path d="M6 12.5h12" fill="none" stroke="currentColor" strokeLinecap="round" strokeWidth="1.8" />
    </svg>
  );
}

function MaximiseIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <path
        d="M7 7.75h10v8.5H7z"
        fill="none"
        stroke="currentColor"
        strokeLinejoin="round"
        strokeWidth="1.8"
      />
    </svg>
  );
}

function CloseIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <path
        d="M7 7l10 10M17 7 7 17"
        fill="none"
        stroke="currentColor"
        strokeLinecap="round"
        strokeWidth="1.8"
      />
    </svg>
  );
}

function ChevronIcon({ open }: { open: boolean }) {
  return (
    <svg className={open ? "chevron-icon open" : "chevron-icon"} viewBox="0 0 24 24" aria-hidden="true">
      <path d="m7 10 5 5 5-5" fill="none" stroke="currentColor" strokeLinecap="round" strokeWidth="1.8" />
    </svg>
  );
}

type SectionKey = "appChecks" | "setup" | "limits" | "snapshot" | "validate" | "runControl" | "progress" | "results";

function App() {
  const [appState, setAppState] = useState<AppState | null>(null);
  const [config, setConfig] = useState<RunConfigInput>(initialConfig);
  const [prescan, setPrescan] = useState<PrescanResult | null>(null);
  const [prescanProgress, setPrescanProgress] = useState<PrescanProgressPayload | null>(null);
  const [validation, setValidation] = useState<ValidationResult | null>(null);
  const [progress, setProgress] = useState<RunProgressPayload | null>(null);
  const [completed, setCompleted] = useState<RunCompletedPayload | null>(null);
  const [logLines, setLogLines] = useState<string[]>([]);
  const [busy, setBusy] = useState<string>("");
  const [error, setError] = useState<string>("");
  const [info, setInfo] = useState<string>("");
  const [theme, setTheme] = useState<ThemeName>(() => readStoredTheme());
  const [mode, setMode] = useState<ThemeMode>(() => readStoredMode());
  const [prescanRanAt, setPrescanRanAt] = useState<string>("");
  const [validatedFingerprint, setValidatedFingerprint] = useState<string>("");
  const [prescanCancelRequested, setPrescanCancelRequested] = useState(false);
  const [sectionOpen, setSectionOpen] = useState<Record<SectionKey, boolean>>({
    appChecks: true,
    setup: true,
    limits: true,
    snapshot: true,
    validate: true,
    runControl: true,
    progress: true,
    results: true,
  });
  const prescanCancelRequestedRef = useRef(false);

  useEffect(() => {
    GetAppState().then(setAppState).catch((err) => setError(String(err)));

    const progressHandler = (payload: RunProgressPayload) => {
      setProgress(payload);
      if (payload.message) {
        setLogLines((current) => [...current.slice(-11), payload.message]);
      }
    };
    const prescanProgressHandler = (payload: PrescanProgressPayload) => {
      setPrescanProgress(payload);
    };
    const completedHandler = (payload: RunCompletedPayload) => {
      setBusy("");
      setCompleted(payload);
      setInfo(`Run finished: ${payload.reportPath}`);
    };
    const failedHandler = (payload: { message: string }) => {
      setBusy("");
      setError(payload.message);
    };

    EventsOn("prescan:progress", prescanProgressHandler);
    EventsOn("run:progress", progressHandler);
    EventsOn("run:completed", completedHandler);
    EventsOn("run:failed", failedHandler);

    return () => {
      EventsOff("prescan:progress");
      EventsOff("run:progress");
      EventsOff("run:completed");
      EventsOff("run:failed");
    };
  }, []);

  useEffect(() => {
    window.localStorage.setItem(THEME_STORAGE_KEY, theme);
  }, [theme]);

  useEffect(() => {
    window.localStorage.setItem(MODE_STORAGE_KEY, mode);
  }, [mode]);

  const dependencies = useMemo(
    () => validation?.dependencies ?? prescan?.dependencies ?? appState?.dependencies ?? [],
    [appState?.dependencies, prescan?.dependencies, validation?.dependencies],
  );
  const environmentChecks = useMemo(
    () => validation?.environment ?? appState?.environment ?? [],
    [appState?.environment, validation?.environment],
  );
  const displayedEnvironmentChecks = useMemo(
    () =>
      environmentChecks.map((item) => {
        if (validation) {
          return item;
        }
        if (item.key === "source_root" && config.sourceDir) {
          return {
            ...item,
            ok: true,
            checked: true,
            status: "Selected",
            detail: "Source root selected. Validate to verify read access.",
          };
        }
        if (item.key === "output_root" && config.outputDir) {
          return {
            ...item,
            ok: true,
            checked: true,
            status: "Selected",
            detail: "Output root selected. Validate to verify write access.",
          };
        }
        return item;
      }),
    [config.outputDir, config.sourceDir, environmentChecks, validation],
  );
  const effectivePrescan = prescan;
  const progressTotalHits = (progress?.filenameHits ?? 0) + (progress?.folderHits ?? 0) + (progress?.contentHits ?? 0);
  const progressPercent =
    progress && progress.totalFiles > 0 ? Math.min(100, Math.round((Math.max(progress.fileNum, 0) / progress.totalFiles) * 100)) : 0;
  const keywordHitStats = progress?.keywordHitStats ?? [];
  const configFingerprint = useMemo(() => JSON.stringify(config), [config]);
  const validationCurrent = validation !== null && validatedFingerprint === configFingerprint;
  const startDisabled = busy !== "" || !validationCurrent || !validation?.ready;
  const isPrescanning = busy === "Prescanning source…";
  const toggleSection = (key: SectionKey) => {
    setSectionOpen((current) => ({ ...current, [key]: !current[key] }));
  };

  const updateConfig = <K extends keyof RunConfigInput>(key: K, value: RunConfigInput[K]) => {
    setConfig((current) => ({ ...current, [key]: value }));
    setValidation(null);
    setValidatedFingerprint("");
    if (key === "sourceDir") {
      setPrescan(null);
      setPrescanProgress(null);
      setPrescanRanAt("");
    }
  };

  const handlePickSource = async () => {
    setError("");
    try {
      const chosen = (await PickSourceDirectory()).trim();
      if (chosen) {
        updateConfig("sourceDir", chosen);
        setInfo(`Source selected: ${chosen}`);
      }
    } catch (err) {
      setError(String(err));
    }
  };

  const handlePickOutput = async () => {
    setError("");
    try {
      const chosen = (await PickOutputDirectory()).trim();
      if (chosen) {
        updateConfig("outputDir", chosen);
        setInfo(`Output selected: ${chosen}`);
      }
    } catch (err) {
      setError(String(err));
    }
  };

  const handlePickKeywordsFile = async () => {
    setError("");
    try {
      const chosen = (await PickKeywordsFile()).trim();
      if (chosen) {
        updateConfig("keywordsFile", chosen);
        setInfo(`Keywords file selected: ${chosen}`);
      }
    } catch (err) {
      setError(String(err));
    }
  };

  const handlePrescan = async () => {
    if (!config.sourceDir) {
      setError("Choose a source directory first.");
      return;
    }
    setBusy("Prescanning source…");
    setPrescanProgress(null);
    prescanCancelRequestedRef.current = false;
    setPrescanCancelRequested(false);
    setError("");
    try {
      const result = await PrescanSource(config.sourceDir);
      setPrescan(result);
      setPrescanRanAt(formatStatusTimestamp(new Date()));
      setInfo(prescanCancelRequestedRef.current ? "Prescan canceled." : "Prescan complete.");
    } catch (err) {
      if (prescanCancelRequestedRef.current || String(err).toLowerCase().includes("canceled")) {
        setInfo("Prescan canceled.");
      } else {
        setError(String(err));
      }
    } finally {
      setBusy("");
      prescanCancelRequestedRef.current = false;
      setPrescanCancelRequested(false);
    }
  };

  const handleCancelPrescan = async () => {
    prescanCancelRequestedRef.current = true;
    setPrescanCancelRequested(true);
    setInfo("Canceling prescan…");
    try {
      await CancelPrescan();
    } catch (err) {
      setError(String(err));
    }
  };

  const handleValidate = async () => {
    setBusy("Validating run…");
    setError("");
    setInfo("");
    try {
      const result = await ValidateRun(config);
      setValidation(result);
      setValidatedFingerprint(configFingerprint);
      if (result.conflicts.length > 0) {
        setInfo("Choose one term for each duplicate normalized keyword.");
      } else if (result.ready) {
        setInfo("Run configuration is ready.");
      }
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy("");
    }
  };

  const handleInstall = async (dependency: string) => {
    setBusy(`Installing ${dependency}…`);
    setError("");
    try {
      const result = await InstallDependency({ dependency });
      if (result.success) {
        setInfo(result.message);
      } else {
        setError(`${result.message}. ${result.reason || result.manualCommand}`);
      }
      const refreshed = await ValidateRun(config);
      setValidation(refreshed);
      setValidatedFingerprint(configFingerprint);
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy("");
    }
  };

  const handleStart = async () => {
    setBusy(config.estimateMode ? "Starting estimate run…" : "Starting scan…");
    setError("");
    setCompleted(null);
    setProgress(null);
    setLogLines([]);
    try {
      const validated = await ValidateRun(config);
      setValidation(validated);
      setValidatedFingerprint(configFingerprint);
      if (!validated.ready) {
        setBusy("");
        setError("Resolve the validation issues before starting the run.");
        return;
      }
      await StartRun(config);
    } catch (err) {
      setBusy("");
      setError(String(err));
    }
  };

  const artifactButtons = completed
    ? [
        { label: "Open Run Folder", path: completed.runDir },
        { label: "Open Report", path: completed.reportPath },
        { label: "Open Review XLSX", path: completed.reviewWorkbookPath },
        { label: "Open Review CSV", path: completed.reviewManifestPath },
        { label: "Open Technical XLSX", path: completed.manifestWorkbookPath },
        { label: "Open Technical CSV", path: completed.manifestPath },
        { label: "Open Inventory CSV", path: completed.inventoryPath },
        { label: "Open JSON", path: completed.configPath },
        { label: "Open Log", path: completed.logPath },
      ]
    : [];

  return (
    <div className="shell" data-theme={theme} data-mode={mode}>
      <div className="window-chrome">
        <div className="window-drag-strip" />
        <div className="window-controls" role="group" aria-label="Window controls">
          <button type="button" className="window-button" onClick={() => WindowMinimise()} aria-label="Minimise window">
            <MinimiseIcon />
          </button>
          <button
            type="button"
            className="window-button"
            onClick={() => WindowToggleMaximise()}
            aria-label="Toggle maximise window"
          >
            <MaximiseIcon />
          </button>
          <button type="button" className="window-button danger" onClick={() => Quit()} aria-label="Close window">
            <CloseIcon />
          </button>
        </div>
      </div>
      <header className="hero">
        <div>
          <p className="eyebrow">Traceable Non-Email Search Console</p>
          <h1>FILEHOG</h1>
          <p className="hero-tagline">Root through the pile</p>
        </div>
        <div className="hero-card appearance-card">
          <span className="appearance-kicker">Appearance</span>
          <div className="theme-stack">
            <div className="theme-pane">
              <span className="hero-stat-label">Theme</span>
              <select value={theme} onChange={(e) => setTheme(e.target.value as ThemeName)} aria-label="Theme family">
                {themeOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </div>

            <div className="mode-pane">
              <span className="hero-stat-label">Mode</span>
              <div className="mode-switch" role="group" aria-label="Color mode">
                <button
                  type="button"
                  className={mode === "light" ? "mode-button active" : "mode-button"}
                  onClick={() => setMode("light")}
                  aria-label="Light mode"
                  title="Light mode"
                >
                  <SunIcon />
                  <span className="sr-only">Light mode</span>
                </button>
                <button
                  type="button"
                  className={mode === "dark" ? "mode-button active" : "mode-button"}
                  onClick={() => setMode("dark")}
                  aria-label="Dark mode"
                  title="Dark mode"
                >
                  <MoonIcon />
                  <span className="sr-only">Dark mode</span>
                </button>
              </div>
            </div>
          </div>
        </div>
      </header>

      <main className="stacked-layout">
        <section className="panel flow-panel collapsible-panel">
          <div className="panel-header collapsible-panel-header">
            <div>
              <p className="panel-kicker">App Checks</p>
              <h2>Dependencies And Environment</h2>
            </div>
            <button type="button" className="section-toggle" onClick={() => toggleSection("appChecks")} aria-expanded={sectionOpen.appChecks}>
              <span>{sectionOpen.appChecks ? "Collapse" : "Expand"}</span>
              <ChevronIcon open={sectionOpen.appChecks} />
            </button>
          </div>
          {sectionOpen.appChecks && (
            <div className="section-body">
              <div className="check-section">
                <h3>Dependencies</h3>
                <div className="dependency-list">
                  {dependencies.map((dep) => (
                    <article key={dep.key} className={`dependency-card ${dep.available ? "ok" : "warn"}`}>
                      <div>
                        <strong>{dep.name}</strong>
                        <span className={`corner-status ${dep.available ? "available" : "missing"}`}>
                          {dep.available ? "Available" : "Missing"}
                        </span>
                        <p>{dep.available ? "Ready for searches." : dep.reason || "Unavailable"}</p>
                        {!dep.available && <code>{dep.installHint}</code>}
                      </div>
                      {!dep.available && dep.autoInstall && (
                        <button type="button" onClick={() => handleInstall(dep.key)} disabled={busy !== ""}>
                          Install
                        </button>
                      )}
                    </article>
                  ))}
                </div>
              </div>

              <div className="check-section">
                <h3>Environment</h3>
                <p className="section-note">Quick machine and path checks for easy troubleshooting.</p>
                <div className="environment-list">
                  {displayedEnvironmentChecks.map((item) => (
                    <article key={item.key} className={`environment-card ${item.ok ? "ok" : item.checked ? "warn" : "pending-card"}`}>
                      <strong>{item.name}</strong>
                      <span className={`corner-status ${item.ok ? "available" : item.checked ? "missing" : "pending"}`}>{item.status}</span>
                      <p>{item.detail}</p>
                    </article>
                  ))}
                </div>
              </div>
            </div>
          )}
        </section>

        <section className="panel setup-panel collapsible-panel">
          <div className="panel-header collapsible-panel-header">
            <div>
              <p className="panel-kicker">Setup</p>
              <h2>Configure The Document Scan</h2>
            </div>
            <button type="button" className="section-toggle" onClick={() => toggleSection("setup")} aria-expanded={sectionOpen.setup}>
              <span>{sectionOpen.setup ? "Collapse" : "Expand"}</span>
              <ChevronIcon open={sectionOpen.setup} />
            </button>
          </div>
          {sectionOpen.setup && (
            <div className="section-body">
              <div className="field-row">
                <label className="field-shell">
                  <span>Source Root</span>
                  <div className="field-control-row">
                    <input value={config.sourceDir} onChange={(e) => updateConfig("sourceDir", e.target.value)} />
                    <button type="button" className="browse-button" onClick={handlePickSource}>
                      Browse
                    </button>
                  </div>
                  <em className={config.sourceDir ? "field-state selected" : "field-state"}>
                    {config.sourceDir ? "Selected" : "No source selected"}
                  </em>
                </label>
              </div>

              <div className="field-row">
                <label className="field-shell">
                  <span>Output Root</span>
                  <div className="field-control-row">
                    <input value={config.outputDir} onChange={(e) => updateConfig("outputDir", e.target.value)} />
                    <button type="button" className="browse-button" onClick={handlePickOutput}>
                      Browse
                    </button>
                  </div>
                  <em className={config.outputDir ? "field-state selected" : "field-state"}>
                    {config.outputDir ? "Selected" : "No output selected"}
                  </em>
                </label>
              </div>

              <label className="stacked-field">
                <span>Keywords</span>
                <textarea
                  rows={6}
                  value={config.keywordsText}
                  onChange={(e) => updateConfig("keywordsText", e.target.value)}
                  placeholder='Example State, "Jordan Reed", infrastructure'
                />
              </label>

              <div className="field-row">
                <label className="field-shell">
                  <span>Keywords File</span>
                  <div className="field-control-row">
                    <input value={config.keywordsFile} onChange={(e) => updateConfig("keywordsFile", e.target.value)} />
                    <button type="button" className="browse-button" onClick={handlePickKeywordsFile}>
                      Browse
                    </button>
                  </div>
                  <em className={config.keywordsFile ? "field-state selected" : "field-state"}>
                    {config.keywordsFile ? "Selected" : "No keyword file selected"}
                  </em>
                </label>
              </div>
              <p className="field-hint">
                Filename search checks file names. Content search checks supported document text when local extractors are available. Folder-name fallback only applies when a matching folder contains no content-searchable files and no filename hits.
              </p>

              <div className="date-grid">
                <label className="field-shell">
                  <span>Start Date</span>
                  <input type="date" value={config.startDate} onChange={(e) => updateConfig("startDate", e.target.value)} />
                  <em className={config.startDate ? "field-state selected" : "field-state"}>{config.startDate || "No lower date limit"}</em>
                </label>
                <label className="field-shell">
                  <span>End Date</span>
                  <input type="date" value={config.endDate} onChange={(e) => updateConfig("endDate", e.target.value)} />
                  <em className={config.endDate ? "field-state selected" : "field-state"}>{config.endDate || "No upper date limit"}</em>
                </label>
              </div>
              <p className="field-hint">
                Date filtering excludes only files confidently before the range. Missing or post-range dates are kept as unknown for review.
              </p>

              {busy && <p className="status-line">{busy}</p>}
              {info && <p className="info-line">{info}</p>}
              {error && <p className="error-line">{error}</p>}
            </div>
          )}
        </section>

        <section className="panel collapsible-panel">
          <div className="panel-header collapsible-panel-header">
            <div>
              <p className="panel-kicker">Search Limits</p>
              <h2>Small Test Controls</h2>
            </div>
            <button type="button" className="section-toggle" onClick={() => toggleSection("limits")} aria-expanded={sectionOpen.limits}>
              <span>{sectionOpen.limits ? "Collapse" : "Expand"}</span>
              <ChevronIcon open={sectionOpen.limits} />
            </button>
          </div>
          {sectionOpen.limits && (
            <div className="section-body">
              <div className="field-row search-limits-grid">
                <label className="field-shell">
                  <span>Search Scope</span>
                  <select
                    value={config.searchScope}
                    onChange={(e) => updateConfig("searchScope", e.target.value as RunConfigInput["searchScope"])}
                  >
                    <option value="both">Names + content</option>
                    <option value="paths">Names only</option>
                    <option value="content">Content only</option>
                  </select>
                  <em className="field-state selected">
                    {config.searchScope === "paths"
                      ? "Path-only test"
                      : config.searchScope === "content"
                        ? "Content-only test"
                        : "Full search"}
                  </em>
                </label>
                <label className="field-shell">
                  <span>Max Matched Items</span>
                  <input
                    type="number"
                    min={0}
                    value={config.maxMatches}
                    onChange={(e) => updateConfig("maxMatches", Math.max(0, Number.parseInt(e.target.value || "0", 10)))}
                  />
                  <em className="field-state">{config.maxMatches > 0 ? `Stops after ${config.maxMatches} matches` : "No limit"}</em>
                </label>
                <label className="field-shell">
                  <span>Max Content Size (MiB)</span>
                  <input
                    type="number"
                    min={0}
                    value={config.maxContentMB}
                    onChange={(e) => updateConfig("maxContentMB", Math.max(0, Number.parseInt(e.target.value || "0", 10)))}
                  />
                  <em className="field-state">{config.maxContentMB > 0 ? `Skips content over ${config.maxContentMB} MiB` : "No limit"}</em>
                </label>
              </div>
              <div className="field-row">
                <label className="field-shell slider-field">
                  <span>ZIP Safety Limit (GiB)</span>
                  <div className="slider-readout">
                    <input
                      type="range"
                      min={1}
                      max={30}
                      step={1}
                      value={config.maxZipGB}
                      onChange={(e) => updateConfig("maxZipGB", Number.parseInt(e.target.value, 10))}
                    />
                    <strong>{config.maxZipGB} GiB</strong>
                  </div>
                  <em className="field-state">Stops scanning inside each ZIP after {config.maxZipGB} GiB uncompressed.</em>
                </label>
              </div>
              <p className="field-hint">
                Use path-only plus a small max count for quick IRL checks. Content size limits skip text extraction for oversized PDFs, Office files, and text files. ZIP safety limits prevent very large archives from overwhelming a run.
              </p>
            </div>
          )}
        </section>

        <section className="panel collapsible-panel">
          <div className="panel-header collapsible-panel-header">
            <div>
              <p className="panel-kicker">Snapshot</p>
              <h2>Optional Inventory Snapshot</h2>
            </div>
            <button type="button" className="section-toggle" onClick={() => toggleSection("snapshot")} aria-expanded={sectionOpen.snapshot}>
              <span>{sectionOpen.snapshot ? "Collapse" : "Expand"}</span>
              <ChevronIcon open={sectionOpen.snapshot} />
            </button>
          </div>
          {sectionOpen.snapshot && (
            <div className="section-body">
              <div className={`check-card ${effectivePrescan ? "success-card" : isPrescanning ? "working-card" : ""}`}>
                <strong>Prescan snapshot</strong>
                <span className={`corner-status ${effectivePrescan ? "available" : isPrescanning ? "pending" : "pending"}`}>
                  {effectivePrescan ? "Done" : isPrescanning ? "Running" : "Optional"}
                </span>
                {isPrescanning ? (
                  <div className="prescan-working" role="status" aria-live="polite">
                    <div className="prescan-working-row">
                      <div className="prescan-working-head">
                        <span className="prescan-spinner" aria-hidden="true" />
                        <div>
                          <p className="meta-copy">
                            {prescanCancelRequested
                              ? "Canceling prescan. This may take a moment to stop safely."
                              : prescanProgress?.message || "Prescanning source. Building a hidden inventory snapshot and caching per-file date metadata for the next run."}
                          </p>
                        </div>
                      </div>
                      <button type="button" className="secondary-button compact-button" onClick={handleCancelPrescan} disabled={prescanCancelRequested}>
                        Cancel Prescan
                      </button>
                    </div>
                    {prescanProgress && !prescanCancelRequested && (
                      <>
                        <div className="prescan-progress-row" aria-hidden="true">
                          <div className="prescan-progress-bar">
                            <div className="prescan-progress-bar-fill" />
                          </div>
                        </div>
                        <div className="prescan-stats-grid">
                          <div>
                            <span>Files inventoried</span>
                            <strong>{prescanProgress.filesDiscovered}</strong>
                          </div>
                          <div>
                            <span>Date records cached</span>
                            <strong>{prescanProgress.dateMetadataCached}</strong>
                          </div>
                          <div>
                            <span>Content searchable</span>
                            <strong>{prescanProgress.contentSearchableFiles}</strong>
                          </div>
                          <div>
                            <span>Filename only</span>
                            <strong>{prescanProgress.filenameOnlyFiles}</strong>
                          </div>
                        </div>
                        <p className="meta-copy">
                          Scan bytes: {formatBytes(prescanProgress.scanBytes)}. Total seen: {formatBytes(prescanProgress.totalBytes)}.
                          {prescanProgress.currentFile ? ` Current file: ${prescanProgress.currentFile}` : ""}
                        </p>
                      </>
                    )}
                  </div>
                ) : (
                  <p className="meta-copy">
                    {effectivePrescan
                      ? `Completed ${prescanRanAt}. This hidden snapshot now carries the file inventory and cached per-file date metadata for the next run against this source.`
                      : "Optional. Run a prescan to inventory files, cache per-file date metadata, and speed the next scan."}
                  </p>
                )}
                <div className="action-row flow-actions">
                  <button type="button" className="secondary-button" onClick={handlePrescan} disabled={busy !== ""}>
                    {isPrescanning ? "Prescanning Source" : "Prescan Source"}
                  </button>
                </div>
                {effectivePrescan && (
                  <>
                    <div className="count-grid compact-grid">
                      <div className="count-card">
                        <span>Non-email Files</span>
                        <strong>{effectivePrescan.filesDiscovered - effectivePrescan.ignoredEmailFiles}</strong>
                      </div>
                      <div className="count-card">
                        <span>Content Searchable</span>
                        <strong>{effectivePrescan.contentSearchableFiles}</strong>
                      </div>
                      <div className="count-card">
                        <span>Filename Only</span>
                        <strong>{effectivePrescan.filenameOnlyFiles}</strong>
                      </div>
                      <div className="count-card">
                        <span>Ignored Email</span>
                        <strong>{effectivePrescan.ignoredEmailFiles}</strong>
                      </div>
                    </div>
                    <p className="meta-copy">
                      Scan bytes: {formatBytes(effectivePrescan.scanBytes)}. Content-searchable bytes: {formatBytes(effectivePrescan.contentSearchableBytes)}.
                    </p>
                    {effectivePrescan.topExtensionStats.length > 0 && (
                      <div className="message-block">
                        <h3>Top Extensions</h3>
                        <ul>
                          {effectivePrescan.topExtensionStats.slice(0, 8).map((stat) => (
                            <li key={stat.extension}>
                              <strong>{stat.extension}</strong>: {stat.count} file(s), {formatBytes(stat.sizeBytes)}
                            </li>
                          ))}
                        </ul>
                      </div>
                    )}
                  </>
                )}
              </div>
            </div>
          )}
        </section>

        <section className="panel collapsible-panel">
          <div className="panel-header collapsible-panel-header">
            <div>
              <p className="panel-kicker">Validate</p>
              <h2>Keyword And Run Checks</h2>
            </div>
            <button type="button" className="section-toggle" onClick={() => toggleSection("validate")} aria-expanded={sectionOpen.validate}>
              <span>{sectionOpen.validate ? "Collapse" : "Expand"}</span>
              <ChevronIcon open={sectionOpen.validate} />
            </button>
          </div>
          {sectionOpen.validate && (
            <div className="section-body">
              {validation ? (
                <>
                  <div className={`check-card ${validation.ready ? "success-card" : "warning-card"}`}>
                    <strong>Validation status</strong>
                    <span className={`corner-status ${validation.ready ? "available" : "pending"}`}>
                      {validation.ready ? "Ready" : "Needs work"}
                    </span>
                    <p className="meta-copy">
                      {validation.ready ? "Run configuration passed the current checks." : "Resolve the remaining items below before starting."}
                    </p>
                    <div className="validation-stats">
                      <span>{validation.mergedTerms.length} merged term(s)</span>
                      <span>{validation.warnings.length} warning(s)</span>
                      <span>{validation.errors.length} error(s)</span>
                    </div>
                  </div>

                  {validation.errors.length > 0 && (
                    <div className="message-block error">
                      <h3>Errors</h3>
                      <ul>
                        {validation.errors.map((item) => (
                          <li key={item}>{item}</li>
                        ))}
                      </ul>
                    </div>
                  )}
                  {validation.warnings.length > 0 && (
                    <div className="message-block warning">
                      <h3>Warnings</h3>
                      <ul>
                        {validation.warnings.map((item) => (
                          <li key={item}>{item}</li>
                        ))}
                      </ul>
                    </div>
                  )}
                  {validation.conflicts.length > 0 && (
                    <div className="message-block">
                      <h3>Keyword Conflicts</h3>
                      {validation.conflicts.map((conflict) => (
                        <div key={conflict.normalized} className="conflict-row">
                          <div>
                            <strong>{conflict.normalized}</strong>
                            <p>Only one of these can own this normalized name.</p>
                          </div>
                          <select
                            value={config.conflictSelections[conflict.normalized] ?? ""}
                            onChange={(e) =>
                              updateConfig("conflictSelections", {
                                ...config.conflictSelections,
                                [conflict.normalized]: e.target.value,
                              })
                            }
                          >
                            <option value="">Choose term</option>
                            {conflict.options.map((option) => (
                              <option key={option} value={option}>
                                {option}
                              </option>
                            ))}
                          </select>
                        </div>
                      ))}
                    </div>
                  )}
                  {!validationCurrent && <p className="meta-copy">Run settings changed. Validate again before starting.</p>}
                  <div className="action-row flow-actions">
                    <button type="button" className="secondary-button" onClick={handleValidate} disabled={busy !== ""}>
                      Revalidate
                    </button>
                  </div>
                </>
              ) : (
                <>
                  <div className="check-card">
                    <strong>Validation status</strong>
                    <span className="corner-status pending">Not run</span>
                    <p className="meta-copy">Validate the run to check paths, deduplicated keywords, dependency fallbacks, and occurrence-count reporting.</p>
                  </div>
                  <div className="action-row flow-actions">
                    <button type="button" className="secondary-button" onClick={handleValidate} disabled={busy !== ""}>
                      Validate
                    </button>
                  </div>
                </>
              )}
            </div>
          )}
        </section>

        <section className="panel collapsible-panel">
          <div className="panel-header collapsible-panel-header">
            <div>
              <p className="panel-kicker">Run Control</p>
              <h2>Launch The Scan</h2>
            </div>
            <button type="button" className="section-toggle" onClick={() => toggleSection("runControl")} aria-expanded={sectionOpen.runControl}>
              <span>{sectionOpen.runControl ? "Collapse" : "Expand"}</span>
              <ChevronIcon open={sectionOpen.runControl} />
            </button>
          </div>
          {sectionOpen.runControl && (
            <div className="section-body">
              <div
                className={`check-card launch-card ${
                  validationCurrent && validation?.ready ? "success-card launch-card-ready" : "warning-card launch-card-pending"
                }`}
              >
                <strong>Launch</strong>
                <span className={`corner-status ${validationCurrent && validation?.ready ? "available" : "pending"}`}>
                  {validationCurrent && validation?.ready ? "Ready" : "Waiting"}
                </span>
                <label className="toggle-card run-mode-card">
                  <span className="toggle-card-header">
                    <input
                      type="checkbox"
                      checked={config.estimateMode}
                      onChange={(e) => updateConfig("estimateMode", e.target.checked)}
                    />
                    <strong>Estimate run only</strong>
                  </span>
                  <em>Build reports without copying matched files.</em>
                </label>
                {!validationCurrent && <p className="meta-copy launch-note">Validate the current setup before starting.</p>}
                <div className="action-row flow-actions">
                  <button type="button" className="primary-button launch-button" onClick={handleStart} disabled={startDisabled}>
                    {config.estimateMode ? "Start Estimate" : "Start Scan"}
                  </button>
                </div>
              </div>
            </div>
          )}
        </section>

        <section className="panel progress-panel collapsible-panel">
          <div className="panel-header collapsible-panel-header">
            <div>
              <p className="panel-kicker">Run Progress</p>
              <h2>Live Scanner Feed</h2>
            </div>
            <button type="button" className="section-toggle" onClick={() => toggleSection("progress")} aria-expanded={sectionOpen.progress}>
              <span>{sectionOpen.progress ? "Collapse" : "Expand"}</span>
              <ChevronIcon open={sectionOpen.progress} />
            </button>
          </div>
          {sectionOpen.progress && (
            <div className="section-body">
              <div className="run-progress-bar" aria-label="Run progress">
                <div className="run-progress-bar-fill" style={{ width: `${progressPercent}%` }} />
              </div>
              <p className="meta-copy progress-context">
                {progress?.usedInventorySnapshot
                  ? "Using prescan snapshot for a known file total."
                  : progress?.totalFiles
                    ? "Progress uses the discovered file total for this run."
                    : "Run progress will appear after discovery starts."}
              </p>
              <div className="progress-strip">
                <div>
                  <span>File Progress</span>
                  <strong>
                    {progress?.fileNum ?? 0} / {progress?.totalFiles ?? 0}
                  </strong>
                </div>
                <div>
                  <span>Matched Items</span>
                  <strong>{progress?.matchedFiles ?? 0}</strong>
                </div>
                <div>
                  <span>Total Hit Occurrences</span>
                  <strong>{progressTotalHits}</strong>
                </div>
                <div>
                  <span>Copied Items</span>
                  <strong>{progress?.copiedFiles ?? 0}</strong>
                </div>
              </div>

              <div className="hit-breakdown-grid">
                <div>
                  <span>Filename hits</span>
                  <strong>{progress?.filenameHits ?? 0}</strong>
                </div>
                <div>
                  <span>Content hits</span>
                  <strong>{progress?.contentHits ?? 0}</strong>
                </div>
                <div>
                  <span>Folder-name hits</span>
                  <strong>{progress?.folderHits ?? 0}</strong>
                </div>
              </div>

              <div className="keyword-leaderboard">
                <div className="leaderboard-header">
                  <strong>Keyword Hit Counts</strong>
                  <span>Sorted most to least</span>
                </div>
                {keywordHitStats.length === 0 ? (
                  <p className="meta-copy">Keyword counts will populate as matches are found.</p>
                ) : (
                  <div className="leaderboard-table">
                    {keywordHitStats.slice(0, 12).map((stat) => (
                      <div className="leaderboard-row" key={stat.keyword}>
                        <strong>{stat.keyword}</strong>
                        <span>Total {stat.totalHits}</span>
                        <span>Filename {stat.filenameHits}</span>
                        <span>Content {stat.contentHits}</span>
                        <span>Folder {stat.folderHits}</span>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              <div className="current-card">
                <p>Current file</p>
                <strong>{progress?.currentFile || "Waiting for run start"}</strong>
                <p>Search method</p>
                <strong>{progress?.searchMethod || "—"}</strong>
                {progress?.note && <p>{progress.note}</p>}
              </div>

              <div className="log-block">
                {logLines.length === 0 ? (
                  <p className="meta-copy">Run output will appear here.</p>
                ) : (
                  logLines.map((line, index) => <div key={`${line}-${index}`}>{line}</div>)
                )}
              </div>
            </div>
          )}
        </section>

        <section className="panel results-panel collapsible-panel">
          <div className="panel-header collapsible-panel-header">
            <div>
              <p className="panel-kicker">Run Artifacts</p>
              <h2>Open The Results</h2>
            </div>
            <button type="button" className="section-toggle" onClick={() => toggleSection("results")} aria-expanded={sectionOpen.results}>
              <span>{sectionOpen.results ? "Collapse" : "Expand"}</span>
              <ChevronIcon open={sectionOpen.results} />
            </button>
          </div>
          {sectionOpen.results && (
            <div className="section-body">
              {completed ? (
                <>
                  <div className="progress-strip">
                    <div>
                      <span>Run Folder</span>
                      <strong>{completed.runDir}</strong>
                    </div>
                    <div>
                      <span>Files Scanned</span>
                      <strong>{completed.filesScanned}</strong>
                    </div>
                    <div>
                      <span>Matched Files</span>
                      <strong>{completed.matchedFiles}</strong>
                    </div>
                    <div>
                      <span>Copied Files</span>
                      <strong>{completed.copiedFiles}</strong>
                    </div>
                  </div>
                  <div className="artifact-grid">
                    {artifactButtons.map((artifact) => (
                      <button
                        type="button"
                        key={artifact.label}
                        className="artifact-button"
                        onClick={() => OpenArtifact(completed.outputRoot, artifact.path)}
                      >
                        {artifact.label}
                      </button>
                    ))}
                  </div>
                  {(completed.warnings.length > 0 || completed.errors.length > 0) && (
                    <div className="message-block warning">
                      <h3>Run Notes</h3>
                      <ul>
                        {completed.warnings.map((item) => (
                          <li key={item}>{item}</li>
                        ))}
                        {completed.errors.map((item) => (
                          <li key={item}>{item}</li>
                        ))}
                      </ul>
                    </div>
                  )}
                </>
              ) : (
                <p className="meta-copy">
                  When a run finishes, this panel will open the run folder, report, reviewer XLSX/CSV, technical XLSX/CSV, inventory, config, and log.
                </p>
              )}
            </div>
          )}
        </section>
      </main>
    </div>
  );
}

export default App;
