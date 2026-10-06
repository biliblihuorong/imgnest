/**
 * 统一的字节数展示：1024 进制，自适应 B/KB/MB/GB/TB/PB。
 * 负数、NaN、Infinity 统一输出 "-"；0 输出 "0 B"。
 */
const UNITS = ["B", "KB", "MB", "GB", "TB", "PB"] as const;

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) {
    return "-";
  }
  let value = bytes;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < UNITS.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }
  if (unitIndex === 0) {
    return `${Math.round(value)} B`;
  }
  const text = value >= 100 ? Math.round(value).toString() : value.toFixed(1);
  return `${text} ${UNITS[unitIndex]}`;
}
