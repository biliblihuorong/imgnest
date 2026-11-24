/**
 * index.html 里的启动加载画面（#__app-loading__）。
 * 本模块不依赖 Vue 或任何包，启动失败时也能用。
 */
const LOADING_ID = "__app-loading__";

/** 应用第一屏渲染完成后调用：淡出再移除。重复调用无副作用。 */
export function hideAppLoading(): void {
  const element = document.getElementById(LOADING_ID);
  if (!element || element.classList.contains("is-hidden")) return;
  element.classList.add("is-hidden");
  const remove = () => element.remove();
  element.addEventListener("transitionend", remove, { once: true });
  // 过渡没触发（样式被覆盖、标签页在后台）时兜底移除，避免永远盖住页面。
  window.setTimeout(remove, 600);
}

/** 启动失败时把加载画面换成错误提示和「重新加载」按钮。 */
export function showAppLoadError(text: string, retryText: string): void {
  const element = document.getElementById(LOADING_ID);
  if (!element) {
    // 已经移除了（失败发生在首屏之后），就直接写进 #app。
    const root = document.getElementById("app");
    if (root) root.textContent = text;
    return;
  }
  element.classList.remove("is-hidden");
  element.classList.add("is-error");
  element.setAttribute("role", "alert");
  const message = element.querySelector<HTMLElement>(".app-loading__error");
  if (!message) return;
  message.textContent = text;
  const button = document.createElement("button");
  button.type = "button";
  button.textContent = retryText;
  button.addEventListener("click", () => window.location.reload());
  message.append(document.createElement("br"), button);
}
