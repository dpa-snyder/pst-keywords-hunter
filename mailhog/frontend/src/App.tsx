import { useEffect, useMemo, useState } from "react";
import "./App.css";
import {
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
  required: boolean;
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

type ValidationResult = {
  ready: boolean;
  errors: string[];
  warnings: string[];
  mergedTerms: string[];
  rejectedKeywords: RejectedKeyword[];
  conflicts: ConflictGroup[];
  dependencies: DependencyInfo[];
  environment?: EnvironmentInfo[];
  counts: Record<string, number>;
  flaggedDirs: string[];
};

type PrescanResult = {
  sourceDir: string;
  counts: Record<string, number>;
  flaggedDirs: string[];
  dependencies: DependencyInfo[];
  precheckedTypes: string[];
};

type RunProgressPayload = {
  eventType: string;
  message: string;
  currentFile: string;
  currentKeyword: string;
  fileNum: number;
  totalFiles: number;
  totalHits: number;
  unknownDateCount: number;
  skippedCount: number;
  outputDir: string;
  note: string;
};

type RunCompletedPayload = {
  runTimestamp: string;
  runDir: string;
  reportPath: string;
  manifestPath: string;
  reviewManifestPath: string;
  manifestWorkbookPath: string;
  reviewWorkbookPath: string;
  configPath: string;
  logPath: string;
  totalFiles: number;
  totalHits: number;
  unknownDateCount: number;
  skippedCount: number;
  warnings: string[];
  errors: string[];
  outputRoot: string;
};

type AppState = {
  version: string;
  supportedTypes: string[];
  dependencies: DependencyInfo[];
  environment?: EnvironmentInfo[];
  cliCommand: string;
};

type ThemeName = "rust" | "forest" | "newsprint" | "grey" | "blue";
type ThemeMode = "light" | "dark";

type RunConfigInput = {
  sourceDir: string;
  outputDir: string;
  keywordsText: string;
  keywordsFile: string;
  enableDateFilter: boolean;
  startDate: string;
  endDate: string;
  estimateMode: boolean;
  enablePST: boolean;
  enableOST: boolean;
  enableEML: boolean;
  enableMSG: boolean;
  enableMBOX: boolean;
  conflictSelections: Record<string, string>;
};

const initialConfig: RunConfigInput = {
  sourceDir: "",
  outputDir: "",
  keywordsText: "",
  keywordsFile: "",
  enableDateFilter: false,
  startDate: "",
  endDate: "",
  estimateMode: false,
  enablePST: true,
  enableOST: true,
  enableEML: true,
  enableMSG: true,
  enableMBOX: true,
  conflictSelections: {},
};

const THEME_STORAGE_KEY = "mailhog-theme";
const MODE_STORAGE_KEY = "mailhog-mode";

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

function App() {
  const [appState, setAppState] = useState<AppState | null>(null);
  const [config, setConfig] = useState<RunConfigInput>(initialConfig);
  const [prescan, setPrescan] = useState<PrescanResult | null>(null);
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

  useEffect(() => {
    GetAppState().then(setAppState).catch((err) => setError(String(err)));

    const progressHandler = (payload: RunProgressPayload) => {
      setProgress(payload);
      if (payload.message) {
        setLogLines((current) => [...current.slice(-11), payload.message]);
      }
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

    EventsOn("run:progress", progressHandler);
    EventsOn("run:completed", completedHandler);
    EventsOn("run:failed", failedHandler);

    return () => {
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
  const configFingerprint = useMemo(() => JSON.stringify(config), [config]);
  const validationCurrent = validation !== null && validatedFingerprint === configFingerprint;
  const startDisabled = busy !== "" || !validationCurrent || !validation?.ready;

  const updateConfig = <K extends keyof RunConfigInput>(key: K, value: RunConfigInput[K]) => {
    setConfig((current) => ({ ...current, [key]: value }));
  };

  const handlePickSource = async () => {
    setError("");
    const chosen = await PickSourceDirectory();
    if (chosen) {
      setConfig((current) => ({ ...current, sourceDir: chosen }));
    }
  };

  const handlePickOutput = async () => {
    setError("");
    const chosen = await PickOutputDirectory();
    if (chosen) {
      setConfig((current) => ({ ...current, outputDir: chosen }));
    }
  };

  const handlePickKeywordsFile = async () => {
    setError("");
    const chosen = await PickKeywordsFile();
    if (chosen) {
      setConfig((current) => ({ ...current, keywordsFile: chosen }));
    }
  };

  const handleCopyCLI = async () => {
    const command = appState?.cliCommand ?? "go run ../cmd/keyword-hunter-cli";
    try {
      await navigator.clipboard.writeText(command);
      setInfo("CLI backup command copied.");
      setError("");
    } catch (err) {
      setError(`Failed to copy CLI command: ${String(err)}`);
    }
  };

  const handlePrescan = async () => {
    if (!config.sourceDir) {
      setError("Choose a source directory first.");
      return;
    }
    setBusy("Prescanning source…");
    setError("");
    try {
      const result = await PrescanSource(config.sourceDir);
      setPrescan(result);
      setConfig((current) => ({
        ...current,
        enablePST: result.precheckedTypes.includes("PST"),
        enableOST: result.precheckedTypes.includes("OST"),
        enableEML: result.precheckedTypes.includes("EML"),
        enableMSG: result.precheckedTypes.includes("MSG"),
        enableMBOX: result.precheckedTypes.includes("MBOX"),
      }));
      setPrescanRanAt(formatStatusTimestamp(new Date()));
      setInfo("Prescan complete.");
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy("");
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
          <p className="eyebrow">Traceable Email Search Console</p>
          <h1>MAILHOG</h1>
          <p className="hero-tagline">Sniff deep; Dig less</p>
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

      <main className="grid">
        <div className="main-column">
          <section className="panel setup-panel">
            <div className="panel-header">
              <div>
                <p className="panel-kicker">Run Setup</p>
                <h2>Configure The Scan</h2>
              </div>
              <button className="ghost-button" onClick={handlePrescan} disabled={busy !== ""}>
                Prescan Source
              </button>
            </div>

            <div className="field-row">
              <label className="field-shell">
                <span>Source Root</span>
                <div className="field-control-row">
                  <input value={config.sourceDir} onChange={(e) => updateConfig("sourceDir", e.target.value)} />
                  <button className="browse-button" onClick={handlePickSource}>
                    Browse
                  </button>
                </div>
              </label>
            </div>

            <div className="field-row">
              <label className="field-shell">
                <span>Output Root</span>
                <div className="field-control-row">
                  <input value={config.outputDir} onChange={(e) => updateConfig("outputDir", e.target.value)} />
                  <button className="browse-button" onClick={handlePickOutput}>
                    Browse
                  </button>
                </div>
              </label>
            </div>

            <label className="stacked-field">
              <span>Keywords</span>
              <textarea
                rows={6}
                value={config.keywordsText}
                onChange={(e) => updateConfig("keywordsText", e.target.value)}
                placeholder='Harbor, "Jordan Reed", infrastructure'
              />
            </label>

            <div className="field-row">
              <label className="field-shell">
                <span>Keywords File</span>
                <div className="field-control-row">
                  <input value={config.keywordsFile} onChange={(e) => updateConfig("keywordsFile", e.target.value)} />
                  <button className="browse-button" onClick={handlePickKeywordsFile}>
                    Browse
                  </button>
                </div>
              </label>
            </div>
            <p className="field-hint">One term per line. Quotes are optional.</p>

            <div className="toggle-row">
              <label className="toggle-card">
                <span className="toggle-card-header">
                  <input
                    type="checkbox"
                    checked={config.enableDateFilter}
                    onChange={(e) => updateConfig("enableDateFilter", e.target.checked)}
                  />
                  <strong>Restrict by message date</strong>
                </span>
                <em>Use a start date, end date, or both.</em>
              </label>
            </div>

            <div className={config.enableDateFilter ? "date-grid" : "date-grid disabled"}>
              <label>
                <span>Start Date</span>
                <input
                  value={config.startDate}
                  onChange={(e) => updateConfig("startDate", e.target.value)}
                  placeholder="YYYY-MM-DD"
                  disabled={!config.enableDateFilter}
                />
              </label>
              <label>
                <span>End Date</span>
                <input
                  value={config.endDate}
                  onChange={(e) => updateConfig("endDate", e.target.value)}
                  placeholder="YYYY-MM-DD"
                  disabled={!config.enableDateFilter}
                />
              </label>
            </div>

            <div className="file-type-grid">
              {[
                ["EnablePST", "PST"],
                ["EnableOST", "OST"],
                ["EnableEML", "EML"],
                ["EnableMSG", "MSG"],
                ["EnableMBOX", "MBOX"],
              ].map(([key, label]) => (
                <label key={label} className="type-chip">
                  <span className="type-chip-header">
                    <input
                      type="checkbox"
                      checked={config[key as keyof RunConfigInput] as boolean}
                      onChange={(e) => updateConfig(key as keyof RunConfigInput, e.target.checked as never)}
                    />
                    <strong>{label}</strong>
                  </span>
                  <em>{prescan?.counts[label] ?? 0} found</em>
                </label>
              ))}
            </div>

            {busy && <p className="status-line">{busy}</p>}
            {info && <p className="info-line">{info}</p>}
            {error && <p className="error-line">{error}</p>}
          </section>

          <section
            className={`panel launch-panel ${
              validationCurrent && validation?.ready ? "launch-panel-ready" : "launch-panel-pending"
            }`}
          >
            <div className="panel-header launch-header">
              <div>
                <p className="panel-kicker">Launch</p>
                <h2>Run Control</h2>
              </div>
            </div>
            <div className="launch-row">
              <label className="toggle-card run-mode-card">
                <span className="toggle-card-header">
                  <input
                    type="checkbox"
                    checked={config.estimateMode}
                    onChange={(e) => updateConfig("estimateMode", e.target.checked)}
                  />
                  <strong>Estimate run only</strong>
                </span>
                <em>Count likely output without exporting hits.</em>
              </label>
              <div className="launch-actions">
                {!validationCurrent && (
                  <p className="meta-copy launch-note">
                    Validate the current setup before starting.
                  </p>
                )}
                <button className="primary-button launch-button" onClick={handleStart} disabled={startDisabled}>
                  {config.estimateMode ? "Start Estimate" : "Start Scan"}
                </button>
              </div>
            </div>
          </section>
        </div>

        <section className="panel side-panel">
          <div className="panel-header">
            <div>
              <p className="panel-kicker">App Checks</p>
              <h2>Run Readiness</h2>
            </div>
          </div>
          <div className="sidebar-stack">
            <div className="check-section">
              <h3>CLI Backup</h3>
              <div className="check-card">
                <div className="cli-backup-row">
                  <code>{appState?.cliCommand ?? "go run ../cmd/keyword-hunter-cli"}</code>
                  <button className="copy-button" onClick={handleCopyCLI}>
                    Copy
                  </button>
                </div>
              </div>
            </div>

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
                      <p>{dep.available ? "Ready for scans." : dep.reason || "Unavailable"}</p>
                      {!dep.available && <code>{dep.installHint}</code>}
                    </div>
                    {!dep.available && dep.autoInstall && (
                      <button onClick={() => handleInstall(dep.key)} disabled={busy !== ""}>
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
                {environmentChecks.map((item) => (
                  <article key={item.key} className={`environment-card ${item.ok ? "ok" : item.checked ? "warn" : "pending-card"}`}>
                    <strong>{item.name}</strong>
                    <span
                      className={`corner-status ${
                        item.ok ? "available" : item.checked ? "missing" : "pending"
                      }`}
                    >
                      {item.status}
                    </span>
                    <p>{item.detail}</p>
                  </article>
                ))}
              </div>
            </div>

            <div className="check-section">
              <h3>Snapshot Ran</h3>
              <div className={`check-card ${prescan ? "success-card" : ""}`}>
                <strong>Prescan snapshot</strong>
                <span className={`corner-status ${prescan ? "available" : "pending"}`}>{prescan ? "Done" : "Not yet"}</span>
                <p className="meta-copy">
                  {prescan ? `Completed ${prescanRanAt}.` : "Run a prescan to populate file counts and flag summaries."}
                </p>
                {prescan && (
                  <>
                    <div className="count-grid compact-grid">
                      {Object.entries(prescan.counts).map(([label, count]) => (
                        <div key={label} className="count-card">
                          <span>{label}</span>
                          <strong>{count}</strong>
                        </div>
                      ))}
                    </div>
                    <p className="meta-copy">{prescan.flaggedDirs.length} flagged folder(s) recorded for reporting.</p>
                  </>
                )}
              </div>
            </div>

            <div className="check-section">
              <h3>Validation Keyword And Run Checks</h3>
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
                            <p>Only one of these can own this folder name.</p>
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
                  {!validationCurrent && (
                    <p className="meta-copy">Run settings changed. Validate again before starting.</p>
                  )}
                  <div className="action-row sidebar-actions sidebar-actions-right">
                    <button className="secondary-button" onClick={handleValidate} disabled={busy !== ""}>
                      Revalidate
                    </button>
                  </div>
                </>
              ) : (
                <>
                  <div className="check-card">
                    <strong>Validation status</strong>
                    <span className="corner-status pending">Not run</span>
                    <p className="meta-copy">Validate the run to check paths, deduplicated keywords, and dependency warnings.</p>
                  </div>
                  <div className="action-row sidebar-actions sidebar-actions-right">
                    <button className="secondary-button" onClick={handleValidate} disabled={busy !== ""}>
                      Validate
                    </button>
                  </div>
                </>
              )}
            </div>
          </div>
        </section>

        <section className="panel progress-panel">
          <div className="panel-header">
            <div>
              <p className="panel-kicker">Run Progress</p>
              <h2>Live Scanner Feed</h2>
            </div>
          </div>
          <div className="progress-strip">
            <div>
              <span>Files</span>
              <strong>
                {progress?.fileNum ?? 0}/{progress?.totalFiles ?? 0}
              </strong>
            </div>
            <div>
              <span>Total Hits</span>
              <strong>{progress?.totalHits ?? 0}</strong>
            </div>
            <div>
              <span>Unknown Date</span>
              <strong>{progress?.unknownDateCount ?? 0}</strong>
            </div>
            <div>
              <span>Skipped</span>
              <strong>{progress?.skippedCount ?? 0}</strong>
            </div>
          </div>

          <div className="current-card">
            <p>Current file</p>
            <strong>{progress?.currentFile || "Waiting for run start"}</strong>
            <p>Current keyword</p>
            <strong>{progress?.currentKeyword || "—"}</strong>
          </div>

          <div className="log-block">
            {logLines.length === 0 ? (
              <p className="meta-copy">Run output will appear here.</p>
            ) : (
              logLines.map((line, index) => <div key={`${line}-${index}`}>{line}</div>)
            )}
          </div>
        </section>

        <section className="panel results-panel">
          <div className="panel-header">
            <div>
              <p className="panel-kicker">Run Artifacts</p>
              <h2>Open The Results</h2>
            </div>
          </div>
          {completed ? (
            <>
              <div className="progress-strip">
                <div>
                  <span>Run Folder</span>
                  <strong>{completed.runDir}</strong>
                </div>
                <div>
                  <span>Total Hits</span>
                  <strong>{completed.totalHits}</strong>
                </div>
                <div>
                  <span>Unknown Date</span>
                  <strong>{completed.unknownDateCount}</strong>
                </div>
                <div>
                  <span>Skipped</span>
                  <strong>{completed.skippedCount}</strong>
                </div>
              </div>
              <div className="artifact-grid">
                {artifactButtons.map((artifact) => (
                  <button
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
            <p className="meta-copy">When a run finishes, this panel will open the run folder, report, CSV, JSON config, and log.</p>
          )}
        </section>
      </main>
    </div>
  );
}

export default App;
