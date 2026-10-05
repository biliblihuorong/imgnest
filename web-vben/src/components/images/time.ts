import { $t } from "@vben/locales";
/**
 * 图片页/回收站页共用的时间展示 helper（仅本目录使用；字节格式化用 @/lib/format）。
 */

/** RFC3339 时间戳 → "YYYY-MM-DD HH:mm"（本地时区）；空值/非法值返回 "-"。 */
export function formatDateTime(iso: string | null): string {
  if (!iso) {
    return "-";
  }
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return "-";
  }
  const pad = (value: number): string => String(value).padStart(2, "0");
  return (
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ` +
    `${pad(date.getHours())}:${pad(date.getMinutes())}`
  );
}

/** 回收站剩余保留时间：优先天、其次小时；已到期返回 "即将清理"。 */
export function formatRetention(purgeAt: string | null, now: Date = new Date()): string {
  if (!purgeAt) {
    return "-";
  }
  const target = new Date(purgeAt).getTime();
  if (Number.isNaN(target)) {
    return "-";
  }
  const diffHours = (target - now.getTime()) / 3_600_000;
  if (diffHours <= 0) {
    return $t("user.trash.expired");
  }
  if (diffHours < 1) {
    return $t("user.trash.lessHour");
  }
  const days = Math.floor(diffHours / 24);
  if (days >= 1) {
    return $t("user.trash.days", { count: days });
  }
  return $t("user.trash.hours", { count: Math.floor(diffHours) });
}
