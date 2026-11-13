// @ts-ignore
/* eslint-disable */
import { request } from "@/services/xa/request";

/** 列出自己的知识库和公共知识库 GET /api/v1/knowledge-bases */
export async function listKnowledgeBases(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listKnowledgeBasesParams,
  options?: { signal?: AbortSignal; anonymous?: boolean }
) {
  return request<API.BasePage>("/api/v1/knowledge-bases", {
    method: "GET",
    params: {
      // limit has a default value: 20
      limit: "20",
      ...params,
    },
    ...(options || {}),
  });
}

/** 创建知识库 POST /api/v1/knowledge-bases */
export async function createKnowledgeBase(
  body: API.CreateInput,
  options?: { signal?: AbortSignal; anonymous?: boolean }
) {
  return request<API.Base>("/api/v1/knowledge-bases", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 查看知识库 GET /api/v1/knowledge-bases/${param0} */
export async function getKnowledgeBase(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getKnowledgeBaseParams,
  options?: { signal?: AbortSignal; anonymous?: boolean }
) {
  const { id: param0, ...queryParams } = params;
  return request<API.Base>(`/api/v1/knowledge-bases/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 删除知识库并排队清理 DELETE /api/v1/knowledge-bases/${param0} */
export async function deleteKnowledgeBase(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deleteKnowledgeBaseParams,
  options?: { signal?: AbortSignal; anonymous?: boolean }
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/v1/knowledge-bases/${param0}`, {
    method: "DELETE",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 修改知识库名称和描述 PATCH /api/v1/knowledge-bases/${param0} */
export async function updateKnowledgeBase(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.updateKnowledgeBaseParams,
  body: API.UpdateInput,
  options?: { signal?: AbortSignal; anonymous?: boolean }
) {
  const { id: param0, ...queryParams } = params;
  return request<API.Base>(`/api/v1/knowledge-bases/${param0}`, {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 列出知识库文档及摄取状态 GET /api/v1/knowledge-bases/${param0}/documents */
export async function listKnowledgeDocuments(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listKnowledgeDocumentsParams,
  options?: { signal?: AbortSignal; anonymous?: boolean }
) {
  const { id: param0, ...queryParams } = params;
  return request<API.DocumentPage>(
    `/api/v1/knowledge-bases/${param0}/documents`,
    {
      method: "GET",
      params: {
        // limit has a default value: 20
        limit: "20",
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** 上传文档；同库相同内容返回已有文档 POST /api/v1/knowledge-bases/${param0}/documents */
export async function uploadKnowledgeDocument(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.uploadKnowledgeDocumentParams,
  body: {},
  file?: File,
  options?: { signal?: AbortSignal; anonymous?: boolean }
) {
  const { id: param0, ...queryParams } = params;
  const formData = new FormData();

  if (file) {
    formData.append("file", file);
  }

  Object.keys(body).forEach((ele) => {
    const item = (body as any)[ele];

    if (item !== undefined && item !== null) {
      if (typeof item === "object" && !(item instanceof File)) {
        if (item instanceof Array) {
          item.forEach((f) => formData.append(ele, f || ""));
        } else {
          formData.append(
            ele,
            new Blob([JSON.stringify(item)], { type: "application/json" })
          );
        }
      } else {
        formData.append(ele, item);
      }
    }
  });

  return request<API.Document>(`/api/v1/knowledge-bases/${param0}/documents`, {
    method: "POST",
    params: { ...queryParams },
    data: formData,
    requestType: "form",
    ...(options || {}),
  });
}

/** 查看文档详情和失败原因 GET /api/v1/knowledge-bases/${param0}/documents/${param1} */
export async function getKnowledgeDocument(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getKnowledgeDocumentParams,
  options?: { signal?: AbortSignal; anonymous?: boolean }
) {
  const { id: param0, docId: param1, ...queryParams } = params;
  return request<API.Document>(
    `/api/v1/knowledge-bases/${param0}/documents/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** 删除文档并排队清理 DELETE /api/v1/knowledge-bases/${param0}/documents/${param1} */
export async function deleteKnowledgeDocument(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deleteKnowledgeDocumentParams,
  options?: { signal?: AbortSignal; anonymous?: boolean }
) {
  const { id: param0, docId: param1, ...queryParams } = params;
  return request<any>(`/api/v1/knowledge-bases/${param0}/documents/${param1}`, {
    method: "DELETE",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 分页查看已完成文档的切片及出处 GET /api/v1/knowledge-bases/${param0}/documents/${param1}/chunks */
export async function listKnowledgeChunks(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listKnowledgeChunksParams,
  options?: { signal?: AbortSignal; anonymous?: boolean }
) {
  const { id: param0, docId: param1, ...queryParams } = params;
  return request<API.ChunkPage>(
    `/api/v1/knowledge-bases/${param0}/documents/${param1}/chunks`,
    {
      method: "GET",
      params: {
        // limit has a default value: 20
        limit: "20",
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** 重新摄取失败文档 POST /api/v1/knowledge-bases/${param0}/documents/${param1}/retry */
export async function retryKnowledgeDocument(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.retryKnowledgeDocumentParams,
  options?: { signal?: AbortSignal; anonymous?: boolean }
) {
  const { id: param0, docId: param1, ...queryParams } = params;
  return request<API.Document>(
    `/api/v1/knowledge-bases/${param0}/documents/${param1}/retry`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
