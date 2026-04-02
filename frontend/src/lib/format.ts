export function formatBytes(value: number): string {
  const units = ["B", "KB", "MB", "GB", "TB"];
  if (value <= 0) return "0 B";
  const idx = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  const num = value / 1024 ** idx;
  return `${num.toFixed(num >= 100 ? 0 : 1)} ${units[idx]}`;
}

export function titleCaseCategory(category: string): string {
  switch (category) {
    case "safe":
      return "安全清理";
    case "confirm":
      return "需确认";
    case "manual":
      return "手动清理";
    case "review":
      return "建议检查";
    default:
      return category;
  }
}
