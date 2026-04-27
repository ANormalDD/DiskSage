import { useEffect, useMemo, useRef, useState } from "react";
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
  showScanMeta?: boolean;
  collapsible?: boolean;
  defaultCollapsed?: boolean;
};

type TimelineItem = AnalyzeProgressEvent & {
  order: number;
};

type TurnBucket = {
  reasonings: TimelineItem[];
  texts: TimelineItem[];
  tools: TimelineItem[];
  statuses: TimelineItem[];
};

function statusText(op: TimelineItem): string {
  switch (op.type) {
    case "paused_rate_limit":
      return op.content ? `触发限流：${op.content}` : "触发限流，等待继续";
    case "resume":
      return "继续迭代 LLM 分析";
    case "completed":
      return "分析完成，已生成建议";
    default:
      return op.content || op.type || "收到事件";
  }
}

function eventTone(type: string): string {
  switch (type) {
    case "reasoning":
      return "reasoning";
    case "tool_call":
      return "call";
    case "paused_rate_limit":
      return "warn";
    case "completed":
      return "done";
    default:
      return "plain";
  }
}

function normalizeToolKey(value: string): string {
  return (value || "").trim().toLowerCase();
}

function normalizePathKey(value: string): string {
  const trimmed = (value || "").trim();
  if (!trimmed) return "";
  const slash = trimmed.replace(/\\/g, "/").replace(/\/+/g, "/");
  return slash.replace(/\/$/, "").toLowerCase();
}

function normalizeEvent(op: AnalyzeProgressEvent, order: number): TimelineItem {
  return {
    ...op,
    type: op.type || "",
    tool: op.tool || "",
    path: op.path || "",
    content: op.content || "",
    reason: op.reason || "",
    input: op.input || "",
    output: op.output || "",
    at: op.at || "",
    turn: typeof op.turn === "number" ? op.turn : Number(op.turn) || 0,
    order,
  };
}

function isSameToolCall(call: TimelineItem, result: TimelineItem): boolean {
  if (normalizeToolKey(call.tool) !== normalizeToolKey(result.tool)) {
    return false;
  }

  const callPath = normalizePathKey(call.path);
  const resultPath = normalizePathKey(result.path);
  if (callPath && resultPath) {
    return callPath === resultPath;
  }

  const callInput = (call.input || "").trim();
  const resultInput = (result.input || "").trim();
  if (callInput && resultInput) {
    return callInput === resultInput;
  }

  return true;
}

function findMatchingToolCall(
  turnBuckets: Map<number, TurnBucket>,
  turnOrder: number[],
  result: TimelineItem
): TimelineItem | null {
  let fallback: TimelineItem | null = null;

  for (let i = turnOrder.length - 1; i >= 0; i--) {
    const turn = turnOrder[i];
    const bucket = turnBuckets.get(turn);
    if (!bucket) continue;

    for (let j = bucket.tools.length - 1; j >= 0; j--) {
      const call = bucket.tools[j];
      if (!isSameToolCall(call, result)) {
        continue;
      }
      if (!call.output) {
        return call;
      }
      if (!fallback) {
        fallback = call;
      }
    }
  }

  return fallback;
}

function buildTurns(llmOps: AnalyzeProgressEvent[]): Array<{ turn: number; bucket: TurnBucket }> {
  const turnBuckets = new Map<number, TurnBucket>();
  const turnOrder: number[] = [];

  const getBucket = (turn: number): TurnBucket => {
    if (!turnBuckets.has(turn)) {
      turnBuckets.set(turn, {
        reasonings: [],
        texts: [],
        tools: [],
        statuses: [],
      });
      turnOrder.push(turn);
    }
    return turnBuckets.get(turn)!;
  };

  llmOps.forEach((raw, index) => {
    const op = normalizeEvent(raw, index);
    const bucket = getBucket(op.turn);

    if (op.type === "tool_result") {
      const matched = findMatchingToolCall(turnBuckets, turnOrder, op);
      if (matched) {
        if (op.input && !matched.input) {
          matched.input = op.input;
        }
        if (op.output) {
          matched.output = op.output;
        } else if (op.content && !matched.output) {
          matched.output = op.content;
        }
        if (op.at) {
          matched.at = op.at;
        }
        return;
      }

      bucket.tools.push({
        ...op,
        type: "tool_call",
        content: op.content || `工具 ${op.tool} 执行完成`,
        output: op.output || op.content,
      });
      return;
    }

    if (op.type === "reasoning") {
      if (bucket.reasonings.length === 0) {
        bucket.reasonings.push(op);
      } else {
        const last = bucket.reasonings[bucket.reasonings.length - 1];
        last.reason = (last.reason || "") + (op.reason || "");
      }
      return;
    }
    if (op.type === "assistant_text") {
      if (bucket.texts.length === 0) {
        bucket.texts.push(op);
      } else {
        const last = bucket.texts[bucket.texts.length - 1];
        last.content = (last.content || "") + (op.content || "");
      }
      return;
    }
    if (op.type === "tool_call") {
      bucket.tools.push(op);
      return;
    }

    bucket.statuses.push(op);
  });

  const turns = Array.from(turnBuckets.keys()).sort((a, b) => a - b);
  const sortByOrder = (a: TimelineItem, b: TimelineItem) => a.order - b.order;
  const orderedTurns: Array<{ turn: number; bucket: TurnBucket }> = [];

  for (const turn of turns) {
    const bucket = turnBuckets.get(turn)!;
    bucket.reasonings.sort(sortByOrder);
    bucket.texts.sort(sortByOrder);
    bucket.tools.sort(sortByOrder);
    bucket.statuses.sort(sortByOrder);
    orderedTurns.push({ turn, bucket });
  }

  return orderedTurns;
}

function formatEventTime(at: string): string {
  if (!at) return "";
  const date = new Date(at);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleTimeString();
}

export default function ScanProgress({
  phase,
  progress,
  currentPath,
  dirsSeen,
  filesSeen,
  llmOps,
  canContinue,
  busy,
  onContinue,
  showScanMeta = true,
  collapsible = false,
  defaultCollapsed = false,
}: Props) {
  const listRef = useRef<HTMLDivElement | null>(null);
  const [layoutVersion, setLayoutVersion] = useState(0);
  const turns = useMemo(() => buildTurns(llmOps), [llmOps]);
  const totalEvents = useMemo(
    () => turns.reduce((sum, turn) => sum + turn.bucket.reasonings.length + turn.bucket.texts.length + turn.bucket.tools.length + turn.bucket.statuses.length, 0),
    [turns]
  );

  useEffect(() => {
    const el = listRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, [llmOps.length, totalEvents, layoutVersion]);

  useEffect(() => {
    const el = listRef.current;
    if (!el) return;

    const onToggle = (event: Event) => {
      const target = event.target as HTMLElement | null;
      if (!target || target.tagName.toLowerCase() !== "details") {
        return;
      }
      window.requestAnimationFrame(() => {
        setLayoutVersion((v) => v + 1);
      });
    };

    el.addEventListener("toggle", onToggle, true);
    return () => {
      el.removeEventListener("toggle", onToggle, true);
    };
  }, []);

  const flowBody = (
    <>
      {showScanMeta && (
        <>
          <h2>{phase === "scanning" ? "扫描中" : "分析中"}</h2>
          <div className="scan-bar">
            <div style={{ width: `${progress}%` }} />
          </div>
          <p>{phase === "scanning" ? "正在构建压缩目录树..." : "正在迭代 LLM 清理分析..."} {progress}%</p>
          <p>已扫描目录 {dirsSeen} 个，文件 {filesSeen} 个</p>
          <p>当前：{phase === "scanning" ? (currentPath || "等待扫描事件...") : "等待模型/工具事件..."}</p>
        </>
      )}

      <div ref={listRef} className="scan-live-list agent-flow">
        <h4>实时 LLM 工作流</h4>
        {turns.length === 0 && <p>暂无操作输出</p>}
        {turns.map(({ turn, bucket }) => (
          <section key={`turn-${turn}`} className="agent-turn">
            <div className="agent-turn-title">Turn {turn}</div>

            {bucket.reasonings.length > 0 && (
              <details className="agent-row">
                <summary className="agent-toggle">思考内容</summary>
                <div className="agent-content-stack">
                  {bucket.reasonings.map((item, idx) => (
                    <pre key={`r-${item.at || idx}`} className="agent-pre">{item.reason || item.content}</pre>
                  ))}
                </div>
              </details>
            )}

            {bucket.texts.map((item, idx) => (
              <pre key={`t-${item.at || idx}`} className="agent-pre agent-pre-plain">{item.content}</pre>
            ))}

            {bucket.tools.length > 0 && (
              <details className="agent-row">
                <summary className="agent-toggle">工具调用（{bucket.tools.length}）</summary>
                <div className="agent-tool-list">
                  {bucket.tools.map((item, idx) => (
                    <details key={`tool-${item.at || idx}-${item.tool}`} className="agent-tool-card">
                      <summary className="agent-tool-summary">
                        <span>{item.tool || "未知工具"}{item.path ? ` · ${item.path}` : ""}</span>
                        <span className="agent-tool-time">{formatEventTime(item.at)}</span>
                      </summary>
                      {(item.input || item.output) ? (
                        <div className="agent-tool-io">
                          {item.input && (
                            <>
                              <h5>输入</h5>
                              <pre className="agent-pre">{item.input}</pre>
                            </>
                          )}
                          {item.output && (
                            <>
                              <h5>输出</h5>
                              <pre className="agent-pre">{item.output}</pre>
                            </>
                          )}
                        </div>
                      ) : (
                        <pre className="agent-pre">{item.content || "无额外输出"}</pre>
                      )}
                    </details>
                  ))}
                </div>
              </details>
            )}

            {bucket.statuses.map((item, idx) => (
              <p key={`s-${item.at || idx}`} className={`agent-status tone-${eventTone(item.type)}`}>{statusText(item)}</p>
            ))}
          </section>
        ))}
      </div>

      {canContinue && (
        <div className="continue-row">
          <p>分析已中断，可在网络恢复后继续重试，并从上次进度接续。</p>
          <button className="primary" disabled={busy} onClick={() => void onContinue()}>
            继续重试
          </button>
        </div>
      )}
    </>
  );

  if (collapsible) {
    return (
      <section className="scan-box workflow-box">
        <details className="workflow-details" open={!defaultCollapsed}>
          <summary>LLM 工作流（{totalEvents} 条）</summary>
          {flowBody}
        </details>
      </section>
    );
  }

  return <section className="scan-box">{flowBody}</section>;
}
