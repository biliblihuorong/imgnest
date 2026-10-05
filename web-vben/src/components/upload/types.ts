/**
 * 上传队列的 UI 状态模型（仅前端展示用，不属于后端契约）。
 */
import type { ImageView } from "@/api/upload";

/** 队列项状态：排队 / 上传中 / 成功 / 失败。 */
export type UploadQueueState = "queued" | "uploading" | "success" | "failed";

/** 上传队列单项。字段在入队时一次性建全，便于响应式更新。 */
export interface UploadQueueItem {
  id: number;
  file: File;
  name: string;
  size: number;
  state: UploadQueueState;
  loaded: number;
  total: number;
  error: { code: number; message: string; status: number } | null;
  image: ImageView | null;
}
