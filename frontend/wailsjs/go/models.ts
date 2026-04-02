export namespace models {
	
	export class LLMConfig {
	    provider: string;
	    api_key: string;
	    model: string;
	    base_url: string;
	    max_tokens: number;
	    max_turns: number;
	
	    static createFrom(source: any = {}) {
	        return new LLMConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.api_key = source["api_key"];
	        this.model = source["model"];
	        this.base_url = source["base_url"];
	        this.max_tokens = source["max_tokens"];
	        this.max_turns = source["max_turns"];
	    }
	}
	export class AppConfig {
	    llm: LLMConfig;
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.llm = this.convertValues(source["llm"], LLMConfig);
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
	export class Recommendation {
	    path: string;
	    size: number;
	    category: string;
	    reason: string;
	    clean_method: string;
	    command: string;
	    risk: string;
	
	    static createFrom(source: any = {}) {
	        return new Recommendation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.size = source["size"];
	        this.category = source["category"];
	        this.reason = source["reason"];
	        this.clean_method = source["clean_method"];
	        this.command = source["command"];
	        this.risk = source["risk"];
	    }
	}
	export class CleanRequest {
	    Items: Recommendation[];
	    PermanentDelete: boolean;
	    ConfirmCommands: boolean;
	    RequestedBy: string;
	
	    static createFrom(source: any = {}) {
	        return new CleanRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Items = this.convertValues(source["Items"], Recommendation);
	        this.PermanentDelete = source["PermanentDelete"];
	        this.ConfirmCommands = source["ConfirmCommands"];
	        this.RequestedBy = source["RequestedBy"];
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
	export class ItemCleanResult {
	    Path: string;
	    Success: boolean;
	    Error: string;
	    Freed: number;
	
	    static createFrom(source: any = {}) {
	        return new ItemCleanResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Path = source["Path"];
	        this.Success = source["Success"];
	        this.Error = source["Error"];
	        this.Freed = source["Freed"];
	    }
	}
	export class CleanSummary {
	    // Go type: time
	    StartedAt: any;
	    // Go type: time
	    EndedAt: any;
	    Results: ItemCleanResult[];
	    Freed: number;
	
	    static createFrom(source: any = {}) {
	        return new CleanSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.StartedAt = this.convertValues(source["StartedAt"], null);
	        this.EndedAt = this.convertValues(source["EndedAt"], null);
	        this.Results = this.convertValues(source["Results"], ItemCleanResult);
	        this.Freed = source["Freed"];
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
	export class FileTypeStat {
	    Pattern: string;
	    Count: number;
	    Size: number;
	
	    static createFrom(source: any = {}) {
	        return new FileTypeStat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Pattern = source["Pattern"];
	        this.Count = source["Count"];
	        this.Size = source["Size"];
	    }
	}
	export class DirNode {
	    Path: string;
	    Name: string;
	    Size: number;
	    Children: DirNode[];
	    FileTypes: FileTypeStat[];
	    MarkerLabels: string[];
	    IsFile: boolean;
	    // Go type: time
	    ModTime: any;
	
	    static createFrom(source: any = {}) {
	        return new DirNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Path = source["Path"];
	        this.Name = source["Name"];
	        this.Size = source["Size"];
	        this.Children = this.convertValues(source["Children"], DirNode);
	        this.FileTypes = this.convertValues(source["FileTypes"], FileTypeStat);
	        this.MarkerLabels = source["MarkerLabels"];
	        this.IsFile = source["IsFile"];
	        this.ModTime = this.convertValues(source["ModTime"], null);
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
	
	
	
	export class LLMDebugInfo {
	    raw_output: string;
	    last_error: string;
	    source: string;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new LLMDebugInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.raw_output = source["raw_output"];
	        this.last_error = source["last_error"];
	        this.source = source["source"];
	        this.updated_at = this.convertValues(source["updated_at"], null);
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
	
	export class ScanResult {
	    Root: DirNode;
	    Compressed: string;
	
	    static createFrom(source: any = {}) {
	        return new ScanResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Root = this.convertValues(source["Root"], DirNode);
	        this.Compressed = source["Compressed"];
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
	export class TokenUsage {
	    input_tokens: number;
	    output_tokens: number;
	    total_tokens: number;
	
	    static createFrom(source: any = {}) {
	        return new TokenUsage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.input_tokens = source["input_tokens"];
	        this.output_tokens = source["output_tokens"];
	        this.total_tokens = source["total_tokens"];
	    }
	}
	export class TokenStats {
	    last: TokenUsage;
	    total: TokenUsage;
	    request_count: number;
	
	    static createFrom(source: any = {}) {
	        return new TokenStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.last = this.convertValues(source["last"], TokenUsage);
	        this.total = this.convertValues(source["total"], TokenUsage);
	        this.request_count = source["request_count"];
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

