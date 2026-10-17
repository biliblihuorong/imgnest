import { defineStore } from "pinia";
import { fetchSystemVersion, type SystemVersion } from "@/api/system";

interface SystemState {
  /** 管理端「关于」信息；只有管理员会加载。 */
  version: SystemVersion | null;
  loading: boolean;
  error: unknown;
}

/** 进行中的版本请求：侧栏与关于页同时触发时只发一次。 */
let pending: Promise<void> | null = null;
let pendingToken: object | null = null;

/** 站点版本与更新提醒（管理员菜单上的小红点读这里）。 */
export const useSystemStore = defineStore("system", {
  state: (): SystemState => ({ version: null, loading: false, error: null }),
  getters: {
    updateAvailable: (state) => state.version?.update_available === true,
  },
  actions: {
    /** 已加载过就不再请求；refresh 时要求服务端重新检查。失败保留旧值。 */
    load({ refresh = false }: { refresh?: boolean } = {}): Promise<void> {
      if (!refresh && this.version) return Promise.resolve();
      if (pending && !refresh) return pending;
      const token = {};
      const run = (async () => {
        this.loading = true;
        try {
          this.version = await fetchSystemVersion(refresh);
          this.error = null;
        } catch (error) {
          this.error = error;
        } finally {
          this.loading = false;
          if (pendingToken === token) {
            pending = null;
            pendingToken = null;
          }
        }
      })();
      pendingToken = token;
      pending = run;
      return run;
    },
    /** 退出或换成非管理员账户时清空，避免把上一位管理员的提醒带过去。 */
    clear(): void {
      this.version = null;
      this.error = null;
    },
  },
});
