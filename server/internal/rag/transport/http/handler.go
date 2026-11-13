// Package http 将知识库接口登记到现有 Huma API，统一认证和错误响应。
package http

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"

	"github.com/danielgtaylor/huma/v2"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/framework/config"
	frameworkhttp "linq-bs/server/internal/framework/http"
	"linq-bs/server/internal/rag"
)

// Handler 只负责 HTTP 约定；权限、配额和事务由知识库服务执行。
type Handler struct {
	Knowledge *rag.Service
	Config    config.KnowledgeConfig
}

// 各输入使用具名类型；路径和分页约束同时生成到 OpenAPI。
type ListInput struct {
	Limit  int `query:"limit" default:"20" minimum:"1" maximum:"100"`
	Offset int `query:"offset" default:"0" minimum:"0"`
}
type BaseInput struct {
	ID string `path:"id"`
}
type CreateInput struct{ Body rag.CreateInput }
type UpdateInput struct {
	ID   string `path:"id"`
	Body rag.UpdateInput
}
type DocumentListInput struct {
	ID     string `path:"id"`
	Limit  int    `query:"limit" default:"20" minimum:"1" maximum:"100"`
	Offset int    `query:"offset" default:"0" minimum:"0"`
}
type DocumentInput struct {
	ID         string `path:"id"`
	DocumentID string `path:"docId"`
}
type ChunkListInput struct {
	ID         string `path:"id"`
	DocumentID string `path:"docId"`
	Limit      int    `query:"limit" default:"20" minimum:"1" maximum:"100"`
	Offset     int    `query:"offset" default:"0" minimum:"0"`
}

// UploadForm 接受且只接受一份文件，服务端另行检查内容和扩展名。
type UploadForm struct {
	File huma.FormFile `form:"file" required:"true" doc:"UTF-8 TXT/Markdown 或有文本层的 PDF"`
}
type UploadInput struct {
	ID      string `path:"id"`
	RawBody huma.MultipartFormFiles[UploadForm]
}
type BaseOutput struct{ Body rag.Base }
type BasesOutput struct{ Body rag.BasePage }
type DocumentOutput struct{ Body rag.Document }
type DocumentsOutput struct{ Body rag.DocumentPage }
type ChunksOutput struct{ Body rag.ChunkPage }
type UploadOutput struct {
	Status int
	Body   rag.Document
}

func principal(ctx context.Context) auth.Principal { p, _ := auth.PrincipalFromContext(ctx); return p }
func apiError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	var failure *rag.Fault
	if errors.As(err, &failure) {
		return frameworkhttp.APIError(ctx, failure.Status, failure.Code, errors.New(failure.Message))
	}
	return frameworkhttp.APIError(ctx, 503, "knowledge_unavailable", err)
}

// RegisterRoutes 供业务进程和离线文档导出共用，不执行数据库操作。
func (h *Handler) RegisterRoutes(api huma.API, resolver auth.Resolver) {
	protect := func(next http.Handler) http.Handler { return frameworkhttp.Authenticate(resolver, next) }
	op := func(id, method, path, summary string, status int, bodyLimit int64) huma.Operation {
		operation := frameworkhttp.Operation(api, "knowledge-bases", id, method, path, summary, status, bodyLimit, map[int]string{
			400: "请求无效（invalid_request、invalid_upload、invalid_knowledge_base、unsupported_file_type、invalid_file_type、invalid_utf8、empty_document）",
			401: "未登录（unauthenticated）", 403: "无权维护公共知识库（forbidden）", 404: "知识库或文档不存在（not_found）",
			409: "名称、配额或状态冲突（name_conflict、knowledge_base_quota、document_quota、storage_quota、document_not_failed）",
			503: "知识库暂时不可用（knowledge_unavailable）",
		})
		operation.SkipValidateParams = false
		return frameworkhttp.OperationAuth(operation, protect, false)
	}
	const root = "/api/v1/knowledge-bases"
	huma.Register(api, op("listKnowledgeBases", "GET", root, "列出自己的知识库和公共知识库", 200, 0), h.list)
	huma.Register(api, op("createKnowledgeBase", "POST", root, "创建知识库", 201, 16<<10), h.create)
	huma.Register(api, op("getKnowledgeBase", "GET", root+"/{id}", "查看知识库", 200, 0), h.get)
	huma.Register(api, op("updateKnowledgeBase", "PATCH", root+"/{id}", "修改知识库名称和描述", 200, 16<<10), h.update)
	huma.Register(api, op("deleteKnowledgeBase", "DELETE", root+"/{id}", "删除知识库并排队清理", 204, 0), h.delete)
	huma.Register(api, op("listKnowledgeDocuments", "GET", root+"/{id}/documents", "列出知识库文档及摄取状态", 200, 0), h.documents)
	upload := op("uploadKnowledgeDocument", "POST", root+"/{id}/documents", "上传文档；同库相同内容返回已有文档", 201, h.Config.MaxFileBytes+(64<<10))
	upload.BodyReadTimeout = h.Config.JobTimeout
	upload.Responses["200"] = &huma.Response{Description: "已存在相同内容，不重复入队", Content: map[string]*huma.MediaType{"application/json": {Schema: api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[rag.Document](), true, "Document")}}}
	upload.Responses["413"] = &huma.Response{Description: "文件超过大小限制（file_too_large）", Content: upload.Responses["400"].Content}
	huma.Register(api, upload, h.upload)
	huma.Register(api, op("getKnowledgeDocument", "GET", root+"/{id}/documents/{docId}", "查看文档详情和失败原因", 200, 0), h.document)
	huma.Register(api, op("deleteKnowledgeDocument", "DELETE", root+"/{id}/documents/{docId}", "删除文档并排队清理", 204, 0), h.deleteDocument)
	huma.Register(api, op("retryKnowledgeDocument", "POST", root+"/{id}/documents/{docId}/retry", "重新摄取失败文档", 200, 0), h.retry)
	huma.Register(api, op("listKnowledgeChunks", "GET", root+"/{id}/documents/{docId}/chunks", "分页查看已完成文档的切片及出处", 200, 0), h.chunks)
}

func (h *Handler) list(ctx context.Context, in *ListInput) (*BasesOutput, error) {
	r, e := h.Knowledge.List(ctx, principal(ctx), in.Limit, in.Offset)
	return &BasesOutput{r}, apiError(ctx, e)
}
func (h *Handler) create(ctx context.Context, in *CreateInput) (*BaseOutput, error) {
	r, e := h.Knowledge.Create(ctx, principal(ctx), in.Body)
	return &BaseOutput{r}, apiError(ctx, e)
}
func (h *Handler) get(ctx context.Context, in *BaseInput) (*BaseOutput, error) {
	r, e := h.Knowledge.Get(ctx, principal(ctx), in.ID)
	return &BaseOutput{r}, apiError(ctx, e)
}
func (h *Handler) update(ctx context.Context, in *UpdateInput) (*BaseOutput, error) {
	r, e := h.Knowledge.Update(ctx, principal(ctx), in.ID, in.Body)
	return &BaseOutput{r}, apiError(ctx, e)
}
func (h *Handler) delete(ctx context.Context, in *BaseInput) (*frameworkhttp.NoContent, error) {
	return nil, apiError(ctx, h.Knowledge.Delete(ctx, principal(ctx), in.ID))
}
func (h *Handler) documents(ctx context.Context, in *DocumentListInput) (*DocumentsOutput, error) {
	r, e := h.Knowledge.Documents(ctx, principal(ctx), in.ID, in.Limit, in.Offset)
	return &DocumentsOutput{r}, apiError(ctx, e)
}
func (h *Handler) document(ctx context.Context, in *DocumentInput) (*DocumentOutput, error) {
	r, e := h.Knowledge.GetDocument(ctx, principal(ctx), in.ID, in.DocumentID)
	return &DocumentOutput{r}, apiError(ctx, e)
}
func (h *Handler) deleteDocument(ctx context.Context, in *DocumentInput) (*frameworkhttp.NoContent, error) {
	return nil, apiError(ctx, h.Knowledge.DeleteDocument(ctx, principal(ctx), in.ID, in.DocumentID))
}
func (h *Handler) retry(ctx context.Context, in *DocumentInput) (*DocumentOutput, error) {
	r, e := h.Knowledge.Retry(ctx, principal(ctx), in.ID, in.DocumentID)
	return &DocumentOutput{r}, apiError(ctx, e)
}
func (h *Handler) chunks(ctx context.Context, in *ChunkListInput) (*ChunksOutput, error) {
	r, e := h.Knowledge.Chunks(ctx, principal(ctx), in.ID, in.DocumentID, in.Limit, in.Offset)
	return &ChunksOutput{r}, apiError(ctx, e)
}
func (h *Handler) upload(ctx context.Context, in *UploadInput) (*UploadOutput, error) {
	file := in.RawBody.Data().File
	defer file.Close()
	defer in.RawBody.Form.RemoveAll()
	data, err := io.ReadAll(io.LimitReader(file, h.Config.MaxFileBytes+1))
	if err != nil {
		return nil, apiError(ctx, err)
	}
	d, duplicate, err := h.Knowledge.Upload(ctx, principal(ctx), in.ID, file.Filename, data)
	if err != nil {
		return nil, apiError(ctx, err)
	}
	status := http.StatusCreated
	if duplicate {
		status = http.StatusOK
	}
	return &UploadOutput{Status: status, Body: d}, nil
}
