import type { AnalyzeProgressEvent, AppConfig, CleanSummary, LLMDebugInfo, Recommendation, ScanProgressEvent, ScanResult, TokenStats } from "./types";

type AppBinding = {
  ScanDrive: (drive: string) => Promise<any>;
  AnalyzeLastScan: () => Promise<any>;
  ContinueAnalyzeLastScan: () => Promise<any>;
  CanContinueAnalyze: () => Promise<boolean>;
  Clean: (req: any) => Promise<any>;
  GetConfig: () => Promise<any>;
  SaveConfig: (cfg: AppConfig) => Promise<void>;
  GetTokenStats: () => Promise<any>;
  GetLLMDebugInfo: () => Promise<any>;
};

declare global {
  interface Window {
    go?: {
      main?: {
        App?: AppBinding;
      };
    };
  }
}

function binding(): AppBinding | undefined {
  return window.go?.main?.App;
}

function runtimeBinding(): any {
  return (window as any).runtime;
}

function normalizeAnalyzePayload(raw: any): any {
  const base = raw?.data ?? (Array.isArray(raw) ? raw[0] : raw);
  let payload = base;

  if (typeof payload === "string") {
    try {
      payload = JSON.parse(payload);
    } catch {
      return {};
    }
  }

  if (!payload || typeof payload !== "object") {
    return {};
  }

  if (payload.data && typeof payload.data === "object") {
    return payload.data;
  }

  return payload;
}

export async function scanDrive(drive: string): Promise<ScanResult> {
  const app = binding();
  if (!app) {
    return {
      compressed: `${drive}\\\n|-- [5.3GB] temp/\n|-- [3.8GB] .gradle/caches/\n|-- [2.1GB] Downloads/*.zip\n\`-- ... (8 more dirs, total 4.3GB)`,
    };
  }
  const res = await app.ScanDrive(drive);
  return {
    compressed: res.Compressed ?? "",
  };
}

export async function analyzeLastScan(): Promise<Recommendation[]> {
  const app = binding();
  if (!app) {
    return [
      {
        path: "D:/temp",
        size: 5.3 * 1024 * 1024 * 1024,
        category: "safe",
        reason: "临时文件可安全清理",
        clean_method: "recycle",
        command: "",
        risk: "低风险",
      },
      {
        path: "D:/Downloads/isos",
        size: 3.2 * 1024 * 1024 * 1024,
        category: "confirm",
        reason: "大文件长期未访问",
        clean_method: "redirect",
        command: "",
        risk: "需要确认是否仍需要",
      },
    ];
  }
  const rows = await app.AnalyzeLastScan();
  return rows.map(normalizeRecommendationRow);
}

export async function continueAnalyzeLastScan(): Promise<Recommendation[]> {
  const app = binding();
  if (!app) {
    return analyzeLastScan();
  }
  const rows = await app.ContinueAnalyzeLastScan();
  return rows.map(normalizeRecommendationRow);
}

function normalizeRecommendationRow(r: any): Recommendation {
  const sizeRaw = r?.size ?? r?.Size ?? 0;
  const parsedSize = typeof sizeRaw === "number" ? sizeRaw : Number(sizeRaw) || 0;
  return {
    path: r?.path ?? r?.Path ?? "",
    size: parsedSize,
    category: r?.category ?? r?.Category ?? "review",
    reason: r?.reason ?? r?.Reason ?? "",
    clean_method: r?.clean_method ?? r?.CleanMethod ?? "recycle",
    command: r?.command ?? r?.Command ?? "",
    risk: r?.risk ?? r?.Risk ?? "",
  };
}

export async function canContinueAnalyze(): Promise<boolean> {
  const app = binding();
  if (!app?.CanContinueAnalyze) {
    return false;
  }
  return !!(await app.CanContinueAnalyze());
}

export async function cleanSelected(items: Recommendation[]): Promise<CleanSummary> {
  const app = binding();
  if (!app) {
    return {
      startedAt: new Date().toISOString(),
      endedAt: new Date().toISOString(),
      freed: items.reduce((sum, item) => sum + item.size, 0),
      results: items.map((item) => ({ path: item.path, success: true, error: "", freed: item.size })),
    };
  }
  const req = {
    Items: items.map((item) => ({
      Path: item.path,
      Size: item.size,
      Category: item.category,
      Reason: item.reason,
      CleanMethod: item.clean_method,
      Command: item.command,
      Risk: item.risk,
    })),
    PermanentDelete: false,
    ConfirmCommands: true,
    RequestedBy: "frontend",
  };
  const out = await app.Clean(req);
  return {
    startedAt: out.StartedAt,
    endedAt: out.EndedAt,
    freed: out.Freed,
    results: (out.Results ?? []).map((row: any) => ({
      path: row.Path,
      success: row.Success,
      error: row.Error,
      freed: row.Freed,
    })),
  };
}

export async function getConfig(): Promise<AppConfig> {
  const app = binding();
  if (!app) {
    return {
      llm: {
        provider: "openai",
        api_key: "",
        model: "gpt-4o-mini",
        base_url: "https://api.openai.com/v1",
        max_tokens: 1200,
        max_turns: 6,
      },
    };
  }
  const cfg = await app.GetConfig();
  const llm = cfg?.llm ?? {};
  return {
    llm: {
      provider: llm.provider ?? "openai",
      api_key: llm.api_key ?? "",
      model: llm.model ?? "gpt-4o-mini",
      base_url: llm.base_url ?? "https://api.openai.com/v1",
      max_tokens: llm.max_tokens ?? 1200,
      max_turns: llm.max_turns ?? 6,
    },
  };
}

export async function saveConfig(cfg: AppConfig): Promise<void> {
  const app = binding();
  if (!app) return;
  await app.SaveConfig({
    llm: {
      provider: cfg.llm.provider,
      api_key: cfg.llm.api_key,
      model: cfg.llm.model,
      base_url: cfg.llm.base_url,
      max_tokens: cfg.llm.max_tokens,
      max_turns: cfg.llm.max_turns,
    },
  });
}

export async function getTokenStats(): Promise<TokenStats> {
  const app = binding();
  if (!app) {
    return {
      last: { input_tokens: 0, output_tokens: 0, total_tokens: 0 },
      total: { input_tokens: 0, output_tokens: 0, total_tokens: 0 },
      request_count: 0,
    };
  }

  const raw = await app.GetTokenStats();
  const last = raw?.last ?? {};
  const total = raw?.total ?? {};

  return {
    last: {
      input_tokens: last.input_tokens ?? 0,
      output_tokens: last.output_tokens ?? 0,
      total_tokens: last.total_tokens ?? 0,
    },
    total: {
      input_tokens: total.input_tokens ?? 0,
      output_tokens: total.output_tokens ?? 0,
      total_tokens: total.total_tokens ?? 0,
    },
    request_count: raw?.request_count ?? 0,
  };
}

export async function getLLMDebugInfo(): Promise<LLMDebugInfo> {
  const app = binding();
  if (!app) {
    return {
      raw_output: "",
      last_error: "",
      source: "",
      updated_at: "",
    };
  }

  const raw = await app.GetLLMDebugInfo();
  return {
    raw_output: raw?.raw_output ?? "",
    last_error: raw?.last_error ?? "",
    source: raw?.source ?? "",
    updated_at: raw?.updated_at ?? "",
  };
}

export function subscribeScanProgress(onProgress: (event: ScanProgressEvent) => void): () => void {
  const runtime = runtimeBinding();
  if (!runtime?.EventsOn) {
    return () => undefined;
  }

  const off = runtime.EventsOn("scan:progress", (raw: any) => {
    const event: ScanProgressEvent = {
      path: raw?.Path ?? "",
      dirs_seen: raw?.DirsSeen ?? 0,
      files_seen: raw?.FilesSeen ?? 0,
      bytes_seen: raw?.BytesSeen ?? 0,
      done: raw?.Done ?? false,
    };
    onProgress(event);
  });

  if (typeof off === "function") {
    return off;
  }
  return () => {
    if (runtime?.EventsOff) {
      runtime.EventsOff("scan:progress");
    }
  };
}

export function subscribeAnalyzeProgress(onProgress: (event: AnalyzeProgressEvent) => void): () => void {
  const runtime = runtimeBinding();
  if (!runtime?.EventsOn) {
    return () => undefined;
  }

  const off = runtime.EventsOn("analyze:progress", (raw: any) => {
    const payload = normalizeAnalyzePayload(raw);
    const turnValue = payload?.Turn ?? payload?.turn ?? 0;
    const event: AnalyzeProgressEvent = {
      type: payload?.Type ?? payload?.type ?? "",
      turn: typeof turnValue === "number" ? turnValue : Number(turnValue) || 0,
      tool: payload?.Tool ?? payload?.tool ?? "",
      path: payload?.Path ?? payload?.path ?? "",
      content: payload?.Content ?? payload?.content ?? "",
      at: payload?.At ?? payload?.at ?? "",
    };
    onProgress(event);
  });

  if (typeof off === "function") {
    return off;
  }
  return () => {
    if (runtime?.EventsOff) {
      runtime.EventsOff("analyze:progress");
    }
  };
}
