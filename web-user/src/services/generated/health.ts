// @ts-ignore
/* eslint-disable */
import { request } from "@/services/openapi-request";

/** 检查进程存活 返回值: Error GET /health/live */
export async function healthLive(options?: { [key: string]: any }) {
  return request<API.ApiError>("/health/live", {
    method: "GET",
    ...(options || {}),
  });
}

/** 检查依赖就绪 GET /health/ready */
export async function healthReady(options?: { [key: string]: any }) {
  return request<API.HealthReady>("/health/ready", {
    method: "GET",
    ...(options || {}),
  });
}
