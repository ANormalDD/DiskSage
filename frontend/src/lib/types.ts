export type CleanCategory = "safe" | "confirm" | "manual" | "review";
export type CleanMethod = "delete" | "command" | "recycle" | "redirect";

export interface Recommendation {
  path: string;
  size: number;
  category: CleanCategory;
  reason: string;
  clean_method: CleanMethod;
  command: string;
  risk: string;
}

export interface ScanResult {
  compressed: string;
}

export interface ScanProgressEvent {
  path: string;
  dirs_seen: number;
  files_seen: number;
  bytes_seen: number;
  done: boolean;
}

export interface AnalyzeProgressEvent {
  type: string;
  turn: number;
  tool: string;
  path: string;
  content: string;
  reason: string;
  input: string;
  output: string;
  at: string;
}

export interface ItemCleanResult {
  path: string;
  success: boolean;
  error: string;
  freed: number;
}

export interface CleanSummary {
  startedAt: string;
  endedAt: string;
  results: ItemCleanResult[];
  freed: number;
}

export interface LLMConfig {
  provider: string;
  api_key: string;
  model: string;
  base_url: string;
  max_tokens: number;
  max_turns: number;
  request_timeout_seconds: number;
  enable_streaming: boolean;
  enable_web_search: boolean;
  tavily_api_key: string;
  tavily_base_url: string;
}

export interface AppConfig {
  llm: LLMConfig;
}

export interface TokenUsage {
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
}

export interface TokenStats {
  last: TokenUsage;
  total: TokenUsage;
  request_count: number;
}

export interface LLMDebugInfo {
  raw_output: string;
  last_error: string;
  source: string;
  updated_at: string;
}
