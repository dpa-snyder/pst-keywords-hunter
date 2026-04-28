export namespace main {
	
	export class EnvironmentInfo {
	    key: string;
	    name: string;
	    ok: boolean;
	    status: string;
	    detail: string;
	    checked: boolean;
	
	    static createFrom(source: any = {}) {
	        return new EnvironmentInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.name = source["name"];
	        this.ok = source["ok"];
	        this.status = source["status"];
	        this.detail = source["detail"];
	        this.checked = source["checked"];
	    }
	}
		export class DependencyInfo {
		    key: string;
		    name: string;
		    available: boolean;
		    installHint: string;
		    reason: string;
		    required: boolean;
		    autoInstall: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DependencyInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.name = source["name"];
		        this.available = source["available"];
		        this.installHint = source["installHint"];
		        this.reason = source["reason"];
		        this.required = source["required"];
		        this.autoInstall = source["autoInstall"];
		    }
		}
	export class AppState {
	    version: string;
	    supportedTypes: string[];
	    dependencies: DependencyInfo[];
	    environment: EnvironmentInfo[];
	    cliCommand: string;
	
	    static createFrom(source: any = {}) {
	        return new AppState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.supportedTypes = source["supportedTypes"];
	        this.dependencies = this.convertValues(source["dependencies"], DependencyInfo);
	        this.environment = this.convertValues(source["environment"], EnvironmentInfo);
	        this.cliCommand = source["cliCommand"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class InstallRequest {
	    dependency: string;
	
	    static createFrom(source: any = {}) {
	        return new InstallRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dependency = source["dependency"];
	    }
	}
	export class InstallResult {
	    dependency: string;
	    success: boolean;
	    message: string;
	    manualCommand: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new InstallResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dependency = source["dependency"];
	        this.success = source["success"];
	        this.message = source["message"];
	        this.manualCommand = source["manualCommand"];
	        this.reason = source["reason"];
	    }
	}
	export class PrescanResult {
	    sourceDir: string;
	    counts: Record<string, number>;
	    flaggedDirs: string[];
	    dependencies: DependencyInfo[];
	    precheckedTypes: string[];
	    hasRelevantIssues: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PrescanResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceDir = source["sourceDir"];
	        this.counts = source["counts"];
	        this.flaggedDirs = source["flaggedDirs"];
	        this.dependencies = this.convertValues(source["dependencies"], DependencyInfo);
	        this.precheckedTypes = source["precheckedTypes"];
	        this.hasRelevantIssues = source["hasRelevantIssues"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RunConfigInput {
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
	
	    static createFrom(source: any = {}) {
	        return new RunConfigInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceDir = source["sourceDir"];
	        this.outputDir = source["outputDir"];
	        this.keywordsText = source["keywordsText"];
	        this.keywordsFile = source["keywordsFile"];
	        this.enableDateFilter = source["enableDateFilter"];
	        this.startDate = source["startDate"];
	        this.endDate = source["endDate"];
	        this.estimateMode = source["estimateMode"];
	        this.enablePST = source["enablePST"];
	        this.enableOST = source["enableOST"];
	        this.enableEML = source["enableEML"];
	        this.enableMSG = source["enableMSG"];
	        this.enableMBOX = source["enableMBOX"];
	        this.conflictSelections = source["conflictSelections"];
	    }
	}
	export class RunStarted {
	    started: boolean;
	    mode: string;
	
	    static createFrom(source: any = {}) {
	        return new RunStarted(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.started = source["started"];
	        this.mode = source["mode"];
	    }
	}
	export class ValidationResult {
	    ready: boolean;
	    errors: string[];
	    warnings: string[];
	    mergedTerms: string[];
	    rejectedKeywords: scanner.RejectedKeyword[];
	    conflicts: scanner.ConflictGroup[];
	    dependencies: DependencyInfo[];
	    environment: EnvironmentInfo[];
	    counts: Record<string, number>;
	    flaggedDirs: string[];
	
	    static createFrom(source: any = {}) {
	        return new ValidationResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ready = source["ready"];
	        this.errors = source["errors"];
	        this.warnings = source["warnings"];
	        this.mergedTerms = source["mergedTerms"];
	        this.rejectedKeywords = this.convertValues(source["rejectedKeywords"], scanner.RejectedKeyword);
	        this.conflicts = this.convertValues(source["conflicts"], scanner.ConflictGroup);
	        this.dependencies = this.convertValues(source["dependencies"], DependencyInfo);
	        this.environment = this.convertValues(source["environment"], EnvironmentInfo);
	        this.counts = source["counts"];
	        this.flaggedDirs = source["flaggedDirs"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace scanner {
	
	export class ConflictGroup {
	    normalized: string;
	    options: string[];
	
	    static createFrom(source: any = {}) {
	        return new ConflictGroup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.normalized = source["normalized"];
	        this.options = source["options"];
	    }
	}
	export class RejectedKeyword {
	    requested: string;
	    normalized: string;
	    kept: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new RejectedKeyword(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.requested = source["requested"];
	        this.normalized = source["normalized"];
	        this.kept = source["kept"];
	        this.reason = source["reason"];
	    }
	}

}
