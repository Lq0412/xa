// @ts-ignore
/* eslint-disable */
import { request } from "@/services/xa/request";

/** 登录账号 POST /api/v1/auth/login */
export async function login(
  body: API.CredentialsRequest,
  options?: { signal?: AbortSignal; anonymous?: boolean }
) {
  return request<API.LoginResponse>("/api/v1/auth/login", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 注销会话 POST /api/v1/auth/logout */
export async function logout(options?: {
  signal?: AbortSignal;
  anonymous?: boolean;
}) {
  return request<any>("/api/v1/auth/logout", {
    method: "POST",
    ...(options || {}),
  });
}

/** 获取当前用户 GET /api/v1/auth/me */
export async function getMe(options?: {
  signal?: AbortSignal;
  anonymous?: boolean;
}) {
  return request<API.UserResponse>("/api/v1/auth/me", {
    method: "GET",
    ...(options || {}),
  });
}

/** 注册账号 POST /api/v1/auth/register */
export async function register(
  body: API.CredentialsRequest,
  options?: { signal?: AbortSignal; anonymous?: boolean }
) {
  return request<API.UserResponse>("/api/v1/auth/register", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}
