import { appConfig } from "@/app.config";

/**
 * 当前标签页唯一的 Bearer token 来源，api/client.ts 与 stores/auth.ts 都只经过这里：
 * - 持久化到 localStorage；存储不可用（隐私模式、配额满）时退回内存，只在当前标签页有效。
 * - 其他标签页登录、退出或换账号时（storage 事件）通知订阅者，auth store 据此撤销旧会话。
 * - 会话被清除或替换时在 window 上派发 SESSION_CLEARED_EVENT，页面用它丢弃上一个账号的
 *   私有数据（上传队列、缩略图缓存、搜索建议等），见 onSessionCleared。
 */
export const TOKEN_STORAGE_KEY = `${appConfig.storagePrefix}.token`;
export const SESSION_CLEARED_EVENT = `${appConfig.storagePrefix}:session-cleared`;

let token: string | null | undefined;
let memoryOnly = false;
type Listener = (token: string | null) => void;
const listeners = new Set<Listener>();

function publish(next: string | null, force = false): void {
  const previous = token;
  token = next;
  if (!force && previous === next) return;
  for (const listener of listeners) listener(next);
  // 首次从存储读取匿名→登录的凭证、尚未有 auth store 时，没有旧的私有会话要撤销。
  if (previous !== undefined && (force || previous !== null || listeners.size > 0))
    window.dispatchEvent(new Event(SESSION_CLEARED_EVENT));
}

function readStorage(force = false): void {
  try {
    const stored = localStorage.getItem(TOKEN_STORAGE_KEY);
    memoryOnly = false;
    publish(stored, force && stored === null);
  } catch {
    // 读取被拒绝时，当前标签页已建立的内存会话继续有效。
    token ??= null;
  }
}

export function getSessionToken(): string | null {
  // 有订阅者（auth store 活着）时只读已同步的值，跨标签变化经 storage 事件先撤销旧 UI；
  // 还没有订阅者时（启动、独立调用 API）直接读存储。
  if (!memoryOnly && listeners.size === 0) readStorage();
  return token ?? null;
}

/** 写入会话。force 为 true 时即使值没变也通知订阅者并派发清除事件（退出时用）。 */
export function setSessionToken(next: string | null, force = false): void {
  try {
    if (next === null) localStorage.removeItem(TOKEN_STORAGE_KEY);
    else localStorage.setItem(TOKEN_STORAGE_KEY, next);
    memoryOnly = false;
  } catch {
    memoryOnly = true;
  }
  publish(next, force);
}

function onStorage(event: StorageEvent): void {
  if (event.key !== TOKEN_STORAGE_KEY && event.key !== null) return;
  try {
    if (event.storageArea && event.storageArea !== localStorage) return;
  } catch {
    return;
  }
  // 读取最新值，避免迟到的 StorageEvent 把刚建立的会话回滚。
  readStorage(event.key === null);
}

/** 订阅会话变化；最后一个订阅者退订时撤销跨标签监听。 */
export function subscribeSession(listener: Listener): () => void {
  if (listeners.size === 0) window.addEventListener("storage", onStorage);
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
    if (listeners.size === 0) window.removeEventListener("storage", onStorage);
  };
}

/** 会话被清除或替换时调用 handler；返回取消监听的函数（在 onBeforeUnmount / onScopeDispose 里调用）。 */
export function onSessionCleared(handler: () => void): () => void {
  window.addEventListener(SESSION_CLEARED_EVENT, handler);
  return () => window.removeEventListener(SESSION_CLEARED_EVENT, handler);
}
