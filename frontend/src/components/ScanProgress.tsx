import type { AnalyzeProgressEvent } from "../lib/types";

type Props = {
  phase: "scanning" | "analyzing";
  progress: number;
  currentPath: string;
  dirsSeen: number;
  filesSeen: number;
  llmOps: AnalyzeProgressEvent[];
  canContinue: boolean;
  busy: boolean;
  onContinue: () => Promise<void>;
};

function renderOp(op: AnalyzeProgressEvent): string {
  if (op.type === "tool_call") {
    return `Turn ${op.turn} 调用 ${op.tool}${op.path ? `(${op.path})` : ""}`;
  }
  if (op.type === "tool_result") {
    return `Turn ${op.turn} 完成 ${op.tool}${op.path ? `(${op.path})` : ""}`;
  }
  if (op.type === "paused_rate_limit") {
    return `Turn ${op.turn} 限流暂停：${op.content}`;
  }
  if (op.type === "resume") {
    return `Turn ${op.turn} 恢复迭代`;
  }
  if (op.type === "completed") {
    return `Turn ${op.turn} 分析完成`;
  }
  return `Turn ${op.turn} ${op.content || op.type}`;
}

export default function ScanProgress({ phase, progress, currentPath, dirsSeen, filesSeen, llmOps, canContinue, busy, onContinue }: Props) {
  return (
    <section className="scan-box">
      <h2>{phase === "scanning" ? "扫描中" : "分析中"}</h2>
      <div className="scan-bar">
        <div style={{ width: `${progress}%` }} />
      </div>
      <p>{phase === "scanning" ? "正在构建压缩目录树..." : "正在迭代 LLM 清理分析..."} {progress}%</p>
      <p>已扫描目录 {dirsSeen} 个，文件 {filesSeen} 个</p>
      <p>当前：{phase === "scanning" ? (currentPath || "等待扫描事件...") : "等待模型/工具事件..."}</p>
      <div className="scan-live-list">
        <h4>实时 LLM 操作流</h4>
        {llmOps.length === 0 && <p>暂无操作输出</p>}
        {llmOps.slice().reverse().map((op, idx) => (
          <div key={`${op.at}-${idx}`} className="scan-live-item">
            {renderOp(op)}
          </div>
        ))}
      </div>
      {canContinue && (
        <div className="continue-row">
          <p>检测到限流中断，可在窗口恢复后手动继续迭代。</p>
          <button className="primary" disabled={busy} onClick={() => void onContinue()}>
            Continue
          </button>
        </div>
      )}
    </section>
  );
}
