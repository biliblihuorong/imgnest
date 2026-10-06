import type { Router } from "vue-router";
export function landingPath(role?: string): string {
  return role === "admin" ? "/dashboard" : "/upload";
}
/** Preserve only known, internal deep links; never redirect back into authentication. */
export function safeRedirect(value: unknown, fallback: string, router: Router): string {
  if (
    typeof value !== "string" ||
    !value.startsWith("/") ||
    value.startsWith("//") ||
    /[\\\u0000-\u001f]/.test(value)
  )
    return fallback;
  const resolved = router.resolve(value);
  if (
    !resolved.matched.length ||
    resolved.matched.some((r) => r.path.includes(":pathMatch")) ||
    ["/login", "/register"].includes(resolved.path)
  )
    return fallback;
  return resolved.fullPath;
}
