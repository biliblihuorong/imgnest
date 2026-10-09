/**
 * 前端共用的后端视图类型。
 * 全部派生自 gen:api 生成的 schema（唯一事实来源 docs/openapi.yaml），
 * 禁止手改 schema.d.ts。
 */
import type { components } from "./schema";

/** 用户安全视图，对应 openapi components.schemas.UserView。 */
export type UserView = components["schemas"]["UserView"];

/** 登录成功数据，对应 openapi components.schemas.LoginData。 */
export type LoginData = components["schemas"]["LoginData"];

/** GET /api/site 返回数据（公开，仅含站点名与注册开关），对应 SiteView。 */
export type SiteInfo = components["schemas"]["SiteView"];
/** 服务端扩展提供的登录方式，对应 LoginProvider。 */
export type LoginProvider = components["schemas"]["LoginProvider"];

/** GET /api/policies 返回的单条规则摘要（当前用户组绑定的启用规则），对应 PolicySummary。 */
export type PolicySummary = components["schemas"]["PolicySummary"];
