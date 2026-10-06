/** Provider URLs are fixed adapters, never administrator-controlled input. */
const SCRIPT_URL = "https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit";
export interface TurnstileOptions {
  sitekey: string;
  action: "login" | "register";
  language: "en" | "zh-CN";
  size: "flexible" | "compact";
  "response-field": false;
  retry: "never";
  "refresh-expired": "manual";
  callback: (token: string) => void;
  "expired-callback": () => void;
  "error-callback": () => void;
  "timeout-callback": () => void;
}
export interface TurnstileApi {
  render: (container: HTMLElement, options: TurnstileOptions) => string;
  reset: (widgetId: string) => void;
  remove: (widgetId: string) => void;
}
declare global {
  interface Window {
    turnstile?: TurnstileApi;
  }
}
let loading: Promise<TurnstileApi> | undefined;

/** One script/loading promise per application window, with bounded failure and explicit retry. */
export function loadTurnstile(): Promise<TurnstileApi> {
  if (window.turnstile) return Promise.resolve(window.turnstile);
  if (loading) return loading;
  loading = new Promise<TurnstileApi>((resolve, reject) => {
    const script = document.createElement("script");
    script.src = SCRIPT_URL;
    script.async = true;
    script.defer = true;
    const finish = (failed: boolean) => {
      window.clearTimeout(timer);
      script.removeEventListener("load", onLoad);
      script.removeEventListener("error", onError);
      if (failed || !window.turnstile) {
        script.remove();
        loading = undefined;
        reject(new Error("captcha-script-unavailable"));
      } else resolve(window.turnstile);
    };
    const onLoad = () => finish(false);
    const onError = () => finish(true);
    const timer = window.setTimeout(onError, 15000);
    script.addEventListener("load", onLoad);
    script.addEventListener("error", onError);
    document.head.append(script);
  });
  return loading;
}
