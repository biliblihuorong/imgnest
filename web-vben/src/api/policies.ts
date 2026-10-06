import { request } from "./client";
import type { PolicySummary } from "./types";

/** 当前用户可用的上传规则（组绑定且启用），按 id 升序；空为 []。 */
export function listPolicies(): Promise<PolicySummary[]> {
  return request<PolicySummary[]>("/api/policies");
}
