import { useMemo, useState } from "react";
import type { AnalyzeProgressEvent, CleanSummary, Recommendation, ScanProgressEvent } from "../lib/types";
import { analyzeLastScan, canContinueAnalyze, cleanSelected, continueAnalyzeLastScan, scanDrive, subscribeAnalyzeProgress, subscribeScanProgress } from "../lib/wailsbridge";

export type Stage = "select" | "scanning" | "analyzing" | "results";

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
      setAnalyzeLiveOps((prev) => {
        const next = [...prev, event];
        if (next.length > 80) {
          return next.slice(next.length - 80);
        }
        return next;
      });
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
      const resumable = await canContinueAnalyze();
      if (resumable) {
        setCanContinue(true);
        setStage("analyzing");
        setProgress(94);
        setError(`分析被限流中断：${message || "触发 API rate limit"}。请等待后点击继续迭代。`);
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
    setStage("analyzing");

    const unsubscribeAnalyze = subscribeAnalyzeProgress((event) => {
      if (!event.type && !event.content && !event.tool && !event.path) {
        return;
      }
      setAnalyzeLiveOps((prev) => {
        const next = [...prev, event];
        if (next.length > 80) {
          return next.slice(next.length - 80);
        }
        return next;
      });
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
      const resumable = await canContinueAnalyze();
      if (resumable) {
        setCanContinue(true);
        setStage("analyzing");
        setProgress(95);
        setError(`分析仍处于限流窗口：${message || "请稍后再继续"}`);
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

  function clearError() {
    setError("");
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
    scanTelemetry,
    busy,
    error,
    runScan,
    continueAnalyze,
    toggleItem,
    cleanupNow,
    clearError,
  };
}
