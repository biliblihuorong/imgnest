/**
 * Token 管理接口（/api/tokens），契约见 docs/openapi.yaml。
 * 明文 Token 只在创建响应中出现一次，前端不得把它写进日志或 URL。
 */
import { request } from "./client";
import type { components } from "./schema";

/** Token 元数据视图，对应 openapi components.schemas.TokenView。 */
export type TokenView = components["schemas"]["TokenView"];

/** 创建成功响应，对应 openapi components.schemas.IssuedToken；token 为明文。 */
export type IssuedToken = components["schemas"]["IssuedToken"];

export interface CreateTokenPayload {
  /** Token 名称，必须包含非空白字符。 */
  name: string;
  /** 可选过期时间，RFC3339 UTC；省略或 null 表示永不过期。 */
  expires_at?: string | null;
}

/** 列出当前用户的全部 Token 元数据（不含明文与哈希）；空为 []。 */
export function listTokens(): Promise<TokenView[]> {
  return request<TokenView[]>("/api/tokens");
}

/**
 * 创建 Token；服务端固定 kind=api、abilities=["*"]，客户端不传这两个字段。
 * 响应中的明文 token 只返回这一次。
 */
export function createToken(payload: CreateTokenPayload): Promise<IssuedToken> {
  return request<IssuedToken>("/api/tokens", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

/** 吊销本人 Token，立即失效；他人或不存在的 id 一律返回 403/20003。 */
export function revokeToken(id: number): Promise<null> {
  return request<null>(`/api/tokens/${id}`, { method: "DELETE" });
}
