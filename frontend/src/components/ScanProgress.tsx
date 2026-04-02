import { useEffect, useRef } from "react";
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

function eventLabel(type: string): string {
  switch (type) {
    case "reasoning":
      return "Reasoning";
    case "tool_call":
      return "工具调用";
    case "tool_result":
      return "工具结果";
    case "assistant_text":
      return "模型文本";
    case "paused_rate_limit":
      return "限流暂停";
    case "resume":
      return "继续分析";
    case "completed":
      return "已完成";
    default:
      return "事件";
  }
}

function eventTone(type: string): string {
  switch (type) {
    case "reasoning":
      return "reasoning";
    case "tool_call":
      return "call";
    case "tool_result":
      return "result";
    case "paused_rate_limit":
      return "warn";
    case "completed":
      return "done";
    default:
      return "plain";
  }
}

function renderMain(op: AnalyzeProgressEvent): string {
  if (op.type === "tool_call") {
    return `调用工具 ${op.tool}${op.path ? ` (${op.path})` : ""}`;
  }
  if (op.type === "tool_result") {
    return `工具 ${op.tool} 执行完成${op.path ? ` (${op.path})` : ""}`;
  }
  if (op.type === "paused_rate_limit") {
    return op.content ? `触发限流：${op.content}` : "触发限流，等待继续";
  }
  if (op.type === "resume") {
    return "继续迭代 LLM 分析";
  }
  if (op.type === "completed") {
    return "分析完成，已生成建议";
  }
  if (op.type === "reasoning") {
    return "模型产生推理过程";
  }
  if (op.type === "assistant_text") {
    return "模型返回文本消息";
  }
  return op.content || op.type || "收到事件";
}

function formatEventTime(at: string): string {
  if (!at) return "";
  const date = new Date(at);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleTimeString();
}

export default function ScanProgress({ phase, progress, currentPath, dirsSeen, filesSeen, llmOps, canContinue, busy, onContinue }: Props) {
  const listRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    const el = listRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, [llmOps.length]);

  return (
    <section className="scan-box">
      <h2>{phase === "scanning" ? "扫描中" : "分析中"}</h2>
      <div className="scan-bar">
        <div style={{ width: `${progress}%` }} />
      </div>
      <p>{phase === "scanning" ? "正在构建压缩目录树..." : "正在迭代 LLM 清理分析..."} {progress}%</p>
      <p>已扫描目录 {dirsSeen} 个，文件 {filesSeen} 个</p>
      <p>当前：{phase === "scanning" ? (currentPath || "等待扫描事件...") : "等待模型/工具事件..."}</p>

      <div ref={listRef} className="scan-live-list agent-flow">
        <h4>实时 LLM 工作流</h4>
        {llmOps.length === 0 && <p>暂无操作输出</p>}
        {llmOps.map((op, idx) => {
          const tone = eventTone(op.type);
          const timeText = formatEventTime(op.at);
          const reasonText = op.reason || (op.type === "reasoning" ? op.content : "");
          const hasToolIO = (op.type === "tool_call" || op.type === "tool_result") && (!!op.input || !!op.output);
          const extraText = op.type !== "reasoning" && op.type !== "assistant_text" ? op.content : "";

          return (
            <article key={`${op.at || "no-at"}-${op.type}-${idx}`} className={`scan-live-item timeline-item tone-${tone}`}>
              <div className="timeline-head">
                <span className={`timeline-badge tone-${tone}`}>{eventLabel(op.type)}</span>
                <span className="timeline-meta">Turn {op.turn}{timeText ? ` · ${timeText}` : ""}</span>
              </div>

              <p className="timeline-main">{renderMain(op)}</p>
              {extraText && <p className="timeline-note">{extraText}</p>}

              {reasonText && (
                <details className="timeline-detail">
                  <summary>查看 reasoning</summary>
                  <pre className="timeline-pre">{reasonText}</pre>
                </details>
              )}

              {hasToolIO && (
                <details className="timeline-detail">
                  <summary>查看工具输入/输出</summary>
                  {op.input && (
                    <>
                      <h5>输入</h5>
                      <pre className="timeline-pre">{op.input}</pre>
                    </>
                  )}
                  {op.output && (
                    <>
                      <h5>输出</h5>
                      <pre className="timeline-pre">{op.output}</pre>
                    </>
                  )}
                </details>
              )}

              {op.type === "assistant_text" && op.content && (
                <details className="timeline-detail">
                  <summary>查看模型文本</summary>
                  <pre className="timeline-pre">{op.content}</pre>
                </details>
              )}
            </article>
          );
        })}
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
