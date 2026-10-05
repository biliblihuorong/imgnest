/**
 * 管理后台页面共用的展示 helper：时间、带「不限」语义的容量/频率文案、错误文案。
 * 字节格式化基础能力来自 @/lib/format。
 */
import { $t } from "@vben/locales";
import { formatBytes } from "@/lib/format";

/** RFC3339 时间戳 → "YYYY-MM-DD HH:mm"（本地时区）；空串返回 "-"，非法值原样返回。 */
export function formatDateTime(iso: string, locale = "zh-CN"): string {
  if (iso === "") {
    return "-";
  }
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return iso;
  }
  if (locale === "en-US") {
    return new Intl.DateTimeFormat(locale, {
      year: "numeric",
      month: "short",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      hourCycle: "h23",
    }).format(date);
  }
  const pad = (value: number): string => String(value).padStart(2, "0");
  return (
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ` +
    `${pad(date.getHours())}:${pad(date.getMinutes())}`
  );
}

/** 字节上限展示：0（含负值防御）显示「不限」，其余走 formatBytes。 */
export function formatBytesOrUnlimited(bytes: number, translate: typeof $t = $t): string {
  if (bytes <= 0) {
    return translate("admin.common.unlimited");
  }
  return formatBytes(bytes);
}

/** 每分钟上传次数展示：0 显示「不限」。 */
export function formatUploadPerMin(count: number, translate: typeof $t = $t): string {
  if (count <= 0) {
    return translate("admin.common.unlimited");
  }
  return translate("admin.format.uploadPerMin", { count });
}
