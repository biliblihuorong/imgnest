/**
 * admin-kit 的演示假后端入口（src/api/client.ts 在演示模式下动态加载它）。
 * ImgNest 不提供演示数据：.env 里 VITE_USE_MOCK=false，这里只为让模块解析通过。
 */
export function mockRequest(...request: unknown[]): Promise<never> {
  void request;
  return Promise.reject(new Error("ImgNest has no demo backend; set VITE_USE_MOCK=false"));
}
