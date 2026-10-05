import { ApiError, getToken, notifyUnauthorized } from "./client";

/** Fetch local previews without exposing credentials in an image URL. */
export async function fetchProtectedThumbnail(url: string, signal?: AbortSignal): Promise<Blob> {
  // Reject encoded separators and double encoding before URL normalization.
  // Image keys cannot contain these reserved characters under the path contract.
  if (/[\\\x00-\x20]/.test(url) || /%(?:2f|5c|25)/i.test(url)) throw new ApiError(10001, "", 400);
  let decoded: string;
  try {
    decoded = decodeURIComponent(url);
  } catch {
    throw new ApiError(10001, "", 400);
  }
  if (decoded.split("/").some((part) => part === "." || part === ".."))
    throw new ApiError(10001, "", 400);
  const target = new URL(url, window.location.origin);
  if (
    target.origin !== window.location.origin ||
    !target.pathname.startsWith("/t/") ||
    target.search ||
    target.hash ||
    target.username ||
    target.password
  ) {
    throw new ApiError(10001, "", 400);
  }
  const headers = new Headers();
  const token = getToken();
  if (token) headers.set("Authorization", `Bearer ${token}`);
  let response: Response;
  try {
    response = await fetch(target.pathname, {
      headers,
      signal,
      credentials: "same-origin",
      redirect: "error",
      cache: "no-store",
    });
  } catch (error) {
    if (signal?.aborted) throw error;
    throw new ApiError(-1, "", 0);
  }
  if (!response.ok) {
    let code = response.status === 401 ? 20001 : -1;
    try {
      const envelope: unknown = await response.json();
      if (
        envelope &&
        typeof envelope === "object" &&
        "code" in envelope &&
        typeof envelope.code === "number"
      )
        code = envelope.code;
    } catch {
      /* A failed image response need not be JSON. */
    }
    notifyUnauthorized(code, token, signal);
    throw new ApiError(code, "", response.status);
  }
  return response.blob();
}
