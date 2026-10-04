/**
 * 前端共用的后端视图类型。
 * Auth 相关视图来自 gen:api 生成的 schema（唯一事实来源 docs/openapi.yaml）；
 * site/policies 由并行后端 worker 提供、openapi.yaml 暂未收录，这里手写，
 * 字段以后端契约为准，禁止手改 schema.d.ts。
 */
import type { components } from "./schema";

/** 用户安全视图，对应 openapi components.schemas.UserView。 */
export type UserView = components["schemas"]["UserView"];

/** 登录成功数据，对应 openapi components.schemas.LoginData。 */
export type LoginData = components["schemas"]["LoginData"];

/** GET /api/site 返回数据（公开，仅含站点名与注册开关）。 */
export interface SiteInfo {
  site_name: string;
  register_enabled: boolean;
}

/** GET /api/policies 返回的单条规则摘要（当前用户组绑定的启用规则）。 */
export interface PolicySummary {
  id: number;
  name: string;
}
