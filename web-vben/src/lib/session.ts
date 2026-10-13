/** 当前标签页的唯一 Bearer 来源；持久化不可用时仍保留内存会话。 */
export const TOKEN_STORAGE_KEY = "imgnest.token";

let token: string | null | undefined;
let memoryOnly = false;
type Listener = (token: string | null) => void;
const listeners = new Set<Listener>();

function publish(next: string | null, force = false): void {
  const previous = token;
  token = next;
  if (!force && previous === next) return;
  for (const listener of listeners) listener(next);
  // 首次从存储读取匿名→登录的凭证、尚未有认证 store 时，没有旧私有会话要撤销。
  if (previous !== undefined && (force || previous !== null || listeners.size > 0))
    window.dispatchEvent(new Event("imgnest:session-cleared"));
}

function readStorage(force = false): void {
  try {
    const stored = localStorage.getItem(TOKEN_STORAGE_KEY);
    memoryOnly = false;
    publish(stored, force && stored === null);
  } catch {
    // 存储读取被拒绝时，当前标签页已建立的内存会话继续有效。
    token ??= null;
  }
}

export function getSessionToken(): string | null {
  // 活跃 store 期间只读已同步的会话；跨标签变化经 storage 事件先撤销旧 UI。
  // 尚无 store 时兼容启动读取与独立 API 调用者直接使用 localStorage。
  if (!memoryOnly && listeners.size === 0) readStorage();
  return token ?? null;
}

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

/** 订阅当前会话；最后一个 store 销毁时撤销跨标签监听。 */
export function subscribeSession(listener: Listener): () => void {
  if (listeners.size === 0) window.addEventListener("storage", onStorage);
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
    if (listeners.size === 0) window.removeEventListener("storage", onStorage);
  };
}
