import { useEffect, useMemo, useState } from "react";
import DiskSelector from "./components/DiskSelector";
import ScanProgress from "./components/ScanProgress";
import ResultList from "./components/ResultList";
import CleanupDialog from "./components/CleanupDialog";
import SettingsPanel from "./components/SettingsPanel";
import { formatBytes } from "./lib/format";
import { getConfig, getLLMDebugInfo, getTokenStats, saveConfig } from "./lib/wailsbridge";
import type { AppConfig, LLMDebugInfo, TokenStats } from "./lib/types";
import { useCleanup } from "./hooks/useCleanup";

const defaultConfig: AppConfig = {
  llm: {
    provider: "openai",
    api_key: "",
    model: "gpt-4o-mini",
    base_url: "https://api.openai.com/v1",
    max_tokens: 1200,
    max_turns: 6,
    enable_web_search: false,
    tavily_api_key: "",
    tavily_base_url: "https://api.tavily.com",
  },
};

const defaultTokenStats: TokenStats = {
  last: { input_tokens: 0, output_tokens: 0, total_tokens: 0 },
  total: { input_tokens: 0, output_tokens: 0, total_tokens: 0 },
  request_count: 0,
};

const defaultDebugInfo: LLMDebugInfo = {
  raw_output: "",
  last_error: "",
  source: "",
  updated_at: "",
};

export default function App() {
  const cleanup = useCleanup();
  const [showDialog, setShowDialog] = useState(false);
  const [showSettings, setShowSettings] = useState(false);
  const [config, setConfig] = useState<AppConfig>(defaultConfig);
  const [tokenStats, setTokenStats] = useState<TokenStats>(defaultTokenStats);
  const [llmDebugInfo, setLLMDebugInfo] = useState<LLMDebugInfo>(defaultDebugInfo);
  const [settingsError, setSettingsError] = useState("");

  useEffect(() => {
    getConfig().then(setConfig).catch(() => setConfig(defaultConfig));
  }, []);

  useEffect(() => {
    let mounted = true;
    const refresh = async () => {
      try {
        const stats = await getTokenStats();
        if (mounted) setTokenStats(stats);
      } catch {
        if (mounted) setTokenStats(defaultTokenStats);
      }

      try {
        const info = await getLLMDebugInfo();
        if (mounted) setLLMDebugInfo(info);
      } catch {
        if (mounted) setLLMDebugInfo(defaultDebugInfo);
      }
    };

    void refresh();
    const timer = window.setInterval(() => {
      void refresh();
    }, 1200);

    return () => {
      mounted = false;
      window.clearInterval(timer);
    };
  }, []);

  const totalSelected = useMemo(
    () => cleanup.selectedItems.reduce((sum, item) => sum + item.size, 0),
    [cleanup.selectedItems]
  );

  return (
    <div className="page">
      <header className="topbar">
        <h1>DiskSage</h1>
        <button className="ghost" onClick={() => {
          setSettingsError("");
          setShowSettings((v) => !v);
        }}>
          Settings
        </button>
      </header>

      {cleanup.error && (
        <section className="error-banner">
          <p>{cleanup.error}</p>
          <div className="error-actions">
            {cleanup.elevationRequired && (
              <button className="primary" disabled={cleanup.busy} onClick={() => void cleanup.requestElevationRestart()}>
                以管理员重启
              </button>
            )}
            <button className="ghost" onClick={cleanup.clearError}>关闭提示</button>
          </div>
        </section>
      )}

      <section className="summary-card">
        <h2>Token 用量</h2>
        <p>
          单次实时：输入 {tokenStats.last.input_tokens} / 输出 {tokenStats.last.output_tokens} / 合计 {tokenStats.last.total_tokens}
        </p>
        <p>
          累计总量：输入 {tokenStats.total.input_tokens} / 输出 {tokenStats.total.output_tokens} / 合计 {tokenStats.total.total_tokens}
        </p>
        <p>请求次数：{tokenStats.request_count}</p>
      </section>

      <section className="summary-card">
        <details className="debug-panel">
          <summary>LLM 原始输出（调试）</summary>
          <p>
            来源：{llmDebugInfo.source || "暂无"}
            {llmDebugInfo.updated_at ? ` | 更新时间：${new Date(llmDebugInfo.updated_at).toLocaleString()}` : ""}
          </p>
          {llmDebugInfo.last_error && <p className="error-text">错误：{llmDebugInfo.last_error}</p>}
          <pre className="debug-raw-output">{llmDebugInfo.raw_output || "暂无原始输出。执行一次分析后会显示。"}</pre>
        </details>
      </section>

      {cleanup.stage === "select" && <DiskSelector onStart={cleanup.runScan} busy={cleanup.busy} />}

      {(cleanup.stage === "scanning" || cleanup.stage === "analyzing") && (
        <ScanProgress
          phase={cleanup.stage === "scanning" ? "scanning" : "analyzing"}
          progress={cleanup.progress}
          currentPath={cleanup.scanTelemetry.path}
          dirsSeen={cleanup.scanTelemetry.dirs_seen}
          filesSeen={cleanup.scanTelemetry.files_seen}
          llmOps={cleanup.analyzeLiveOps}
          canContinue={cleanup.canContinue}
          busy={cleanup.busy}
          onContinue={cleanup.continueAnalyze}
        />
      )}

      {cleanup.stage === "results" && (
        <>
          <section className="summary-card">
            <h2>扫描完成</h2>
            <p>已选中可释放 {formatBytes(totalSelected)}</p>
            <button className="primary" onClick={() => setShowDialog(true)} disabled={cleanup.selectedItems.length === 0 || cleanup.busy}>
              清理选中项
            </button>
          </section>

          <ScanProgress
            phase="analyzing"
            progress={100}
            currentPath={cleanup.scanTelemetry.path}
            dirsSeen={cleanup.scanTelemetry.dirs_seen}
            filesSeen={cleanup.scanTelemetry.files_seen}
            llmOps={cleanup.analyzeLiveOps}
            canContinue={cleanup.canContinue}
            busy={cleanup.busy}
            onContinue={cleanup.continueAnalyze}
            showScanMeta={false}
            collapsible
            defaultCollapsed
          />

          <section className="tree-card">
            <h3>压缩目录树</h3>
            <pre>{cleanup.compressedTree}</pre>
          </section>

          <ResultList
            recommendations={cleanup.recommendations}
            selected={cleanup.selected}
            onToggle={cleanup.toggleItem}
          />
        </>
      )}

      {showDialog && (
        <CleanupDialog
          count={cleanup.selectedItems.length}
          totalSize={totalSelected}
          busy={cleanup.busy}
          onCancel={() => setShowDialog(false)}
          onConfirm={async () => {
            await cleanup.cleanupNow();
            setShowDialog(false);
          }}
        />
      )}

      {showSettings && (
        <SettingsPanel
          config={config}
          saveError={settingsError}
          onClose={() => setShowSettings(false)}
          onSave={async (next) => {
            setSettingsError("");
            try {
              await saveConfig(next);
              setConfig(next);
              setShowSettings(false);
            } catch (err) {
              const message = err instanceof Error ? err.message : String(err);
              setSettingsError(message || "配置保存失败");
            }
          }}
        />
      )}

      {cleanup.lastSummary && (
        <footer className="result-footer">最近一次清理释放 {formatBytes(cleanup.lastSummary.freed)}</footer>
      )}
    </div>
  );
}
