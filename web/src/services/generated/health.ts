// @ts-ignore
/* eslint-disable */
import { request } from "@/services/xa/request";

/** 检查进程存活 返回值: Error GET /health/live */
export async function healthLive(options?: {
  signal?: AbortSignal;
  anonymous?: boolean;
}) {
  return request<API.ApiError>("/health/live", {
    method: "GET",
    ...(options || {}),
  });
}

/** 检查依赖就绪 GET /health/ready */
export async function healthReady(options?: {
  signal?: AbortSignal;
  anonymous?: boolean;
}) {
  return request<API.HealthReady>("/health/ready", {
    method: "GET",
    ...(options || {}),
  });
}
