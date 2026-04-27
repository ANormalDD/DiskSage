import { useMemo, useState } from "react";
import type { AnalyzeProgressEvent, CleanSummary, Recommendation, ScanProgressEvent } from "../lib/types";
import { analyzeLastScan, canContinueAnalyze, cleanSelected, continueAnalyzeLastScan, requestElevation, scanDrive, subscribeAnalyzeProgress, subscribeScanProgress } from "../lib/wailsbridge";

export type Stage = "select" | "scanning" | "analyzing" | "results";

const ELEVATION_REQUIRED_TOKEN = "ELEVATION_REQUIRED";

function isElevationRequiredError(message: string): boolean {
  return message.toUpperCase().includes(ELEVATION_REQUIRED_TOKEN);
}

function isRateLimitLikeError(message: string): boolean {
  const text = (message || "").toLowerCase();
  return text.includes("rate limit") || text.includes("too many requests") || text.includes("429");
}

function appendAnalyzeEvent(prev: AnalyzeProgressEvent[], event: AnalyzeProgressEvent): AnalyzeProgressEvent[] {
  const next = [...prev];
  const last = next[next.length - 1];

  const canMerge =
    !!last &&
    last.turn === event.turn &&
    last.type === event.type &&
    (event.type === "assistant_text" || event.type === "reasoning");

  if (canMerge) {
    const merged: AnalyzeProgressEvent = { ...last };
    if (event.type === "assistant_text") {
      merged.content = `${last.content || ""}${event.content || ""}`;
    }
    if (event.type === "reasoning") {
      merged.reason = `${last.reason || ""}${event.reason || ""}`;
    }
    merged.at = event.at || last.at;
    next[next.length - 1] = merged;
  } else {
    next.push(event);
  }

  if (next.length > 80) {
    return next.slice(next.length - 80);
  }
  return next;
}

export function useCleanup() {
  const [stage, setStage] = useState<Stage>("select");
  const [progress, setProgress] = useState(0);
  const [compressedTree, setCompressedTree] = useState("");
  const [recommendations, setRecommendations] = useState<Recommendation[]>([]);
  const [selected, setSelected] = useState<Record<string, boolean>>({});
  const [lastSummary, setLastSummary] = useState<CleanSummary | null>(null);
  const [scanLivePaths, setScanLivePaths] = useState<string[]>([]);
  const [analyzeLiveOps, setAnalyzeLiveOps] = useState<AnalyzeProgressEvent[]>([]);
  const [canContinue, setCanContinue] = useState(false);
  const [elevationRequired, setElevationRequired] = useState(false);
  const [scanTelemetry, setScanTelemetry] = useState<ScanProgressEvent>({
    path: "",
    dirs_seen: 0,
    files_seen: 0,
    bytes_seen: 0,
    done: false,
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function runScan(drive: string) {
    setError("");
    setBusy(true);
    setStage("scanning");
    setProgress(15);
    setScanLivePaths([]);
    setAnalyzeLiveOps([]);
    setCanContinue(false);
    setElevationRequired(false);

    const unsubscribe = subscribeScanProgress((event) => {
      setScanTelemetry(event);
      if (event.path) {
        setScanLivePaths((prev) => {
          if (prev[prev.length - 1] === event.path) return prev;
          const next = [...prev, event.path];
          if (next.length > 30) {
            return next.slice(next.length - 30);
          }
          return next;
        });
      }
      if (event.done) {
        setProgress(90);
      }
    });

    const unsubscribeAnalyze = subscribeAnalyzeProgress((event) => {
      if (!event.type && !event.content && !event.tool && !event.path) {
        return;
      }
      setAnalyzeLiveOps((prev) => appendAnalyzeEvent(prev, event));
    });

    try {
      const scan = await scanDrive(drive);
      setCompressedTree(scan.compressed);
      setProgress(92);
      setStage("analyzing");

      const recs = await analyzeLastScan();
      setRecommendations(recs);
      setSelected(
        Object.fromEntries(
          recs
            .filter((item) => item.category === "safe")
            .map((item) => [item.path, true])
        )
      );
      setProgress(100);
      setStage("results");
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      if (isElevationRequiredError(message)) {
        setElevationRequired(true);
        setError("扫描系统盘需要管理员权限。请点击“以管理员重启”后重试。");
        setStage("select");
        setProgress(0);
        return;
      }
      const resumable = await canContinueAnalyze();
      if (resumable) {
        setCanContinue(true);
        setStage("analyzing");
        setProgress(94);
        if (isRateLimitLikeError(message)) {
          setError(`分析被限流中断：${message || "触发 API rate limit"}。请等待后点击继续迭代。`);
        } else {
          setError(`分析中断：${message || "网络或模型服务异常"}。可点击继续重试，从上次进度继续。`);
        }
      } else {
        setError(message || "扫描失败，请检查配置后重试");
        setStage("select");
        setProgress(0);
      }
    } finally {
      unsubscribe();
      unsubscribeAnalyze();
      setBusy(false);
    }
  }

  async function continueAnalyze() {
    setError("");
    setBusy(true);
    setCanContinue(false);
    setElevationRequired(false);
    setStage("analyzing");

    const unsubscribeAnalyze = subscribeAnalyzeProgress((event) => {
      if (!event.type && !event.content && !event.tool && !event.path) {
        return;
      }
      setAnalyzeLiveOps((prev) => appendAnalyzeEvent(prev, event));
    });

    try {
      const recs = await continueAnalyzeLastScan();
      setRecommendations(recs);
      setSelected(
        Object.fromEntries(
          recs
            .filter((item) => item.category === "safe")
            .map((item) => [item.path, true])
        )
      );
      setProgress(100);
      setStage("results");
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      if (isElevationRequiredError(message)) {
        setElevationRequired(true);
        setError("分析过程中触发管理员权限要求。请点击“以管理员重启”后重试。");
        setStage("select");
        setProgress(0);
        return;
      }
      const resumable = await canContinueAnalyze();
      if (resumable) {
        setCanContinue(true);
        setStage("analyzing");
        setProgress(95);
        if (isRateLimitLikeError(message)) {
          setError(`分析仍处于限流窗口：${message || "请稍后再继续"}`);
        } else {
          setError(`分析继续失败：${message || "网络或模型服务异常"}。可再次点击继续重试。`);
        }
      } else {
        setError(message || "分析继续失败");
        setStage("select");
        setProgress(0);
      }
    } finally {
      unsubscribeAnalyze();
      setBusy(false);
    }
  }

  function toggleItem(path: string) {
    setSelected((prev) => ({ ...prev, [path]: !prev[path] }));
  }

  const selectedItems = useMemo(
    () => recommendations.filter((item) => selected[item.path]),
    [recommendations, selected]
  );

  async function cleanupNow() {
    if (selectedItems.length === 0) return;
    setError("");
    setBusy(true);
    try {
      const summary = await cleanSelected(selectedItems);
      setLastSummary(summary);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      setError(message || "清理失败，请稍后重试");
    } finally {
      setBusy(false);
    }
  }

  async function requestElevationRestart() {
    setBusy(true);
    try {
      const started = await requestElevation();
      if (!started) {
        setError("当前环境不支持自动提权，请手动以管理员身份重新启动应用。");
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      setError(message || "提权请求失败，请手动以管理员身份重新启动应用。");
    } finally {
      setBusy(false);
    }
  }

  function clearError() {
    setError("");
    setElevationRequired(false);
  }

  return {
    stage,
    progress,
    compressedTree,
    recommendations,
    selected,
    selectedItems,
    lastSummary,
    scanLivePaths,
    analyzeLiveOps,
    canContinue,
    elevationRequired,
    scanTelemetry,
    busy,
    error,
    runScan,
    continueAnalyze,
    requestElevationRestart,
    toggleItem,
    cleanupNow,
    clearError,
  };
}
