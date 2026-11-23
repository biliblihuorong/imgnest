/**
 * 演示模式（src/api/mock.ts 的内存假后端）开关：
 * - 开发（pnpm dev / vitest）：默认开启，写 VITE_USE_MOCK=false 才连真实后端。
 * - 生产构建（pnpm build）：默认关闭，只有显式 VITE_USE_MOCK=true 才打出纯前端演示包。
 *
 * 必须直接读 import.meta.env：构建时它被替换成字面量，关闭时 mock.ts（连同演示账号密码）
 * 会被整段摇树去掉，不进产物。不要改成运行时读取的配置。
 * src/api/client.ts 里加载 mock 的地方原样重复了这个条件（跨模块的常量不会被折叠），改这里要同步改那里。
 */
export const MOCK_MODE: boolean = import.meta.env.DEV
  ? import.meta.env.VITE_USE_MOCK !== "false"
  : import.meta.env.VITE_USE_MOCK === "true";

export function useMock(): boolean {
  return MOCK_MODE;
}
