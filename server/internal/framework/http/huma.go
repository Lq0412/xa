package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strconv"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

const bearerScheme = "bearerAuth"

// EmptyInput 用于只从认证上下文获取输入的接口。
type EmptyInput struct{}

// NoContent 表示响应只有 HTTP 状态码。
type NoContent struct{}

// apiError 是 JSON 接口统一使用的错误响应。
type apiError struct {
	Code      string `json:"code" doc:"稳定的错误码"`
	Message   string `json:"message" doc:"错误说明"`
	RequestID string `json:"request_id" doc:"请求标识"`
	status    int
}

func (e *apiError) Error() string  { return e.Message }
func (e *apiError) GetStatus() int { return e.status }

// APIError 保留既有错误格式并隐藏服务端内部错误。
func APIError(ctx context.Context, status int, code string, err error) huma.StatusError {
	return newAPIError(ctx, status, code, err.Error())
}

func newAPIError(ctx context.Context, status int, code, message string) *apiError {
	if status >= http.StatusInternalServerError {
		message = "服务暂时不可用"
	}
	return &apiError{Code: code, Message: message, RequestID: RequestID(ctx), status: status}
}

func init() {
	// Huma 使用无上下文工厂提取错误 Schema，解析失败则使用带上下文的工厂。
	huma.NewError = func(status int, message string, _ ...error) huma.StatusError {
		return &apiError{Message: message, status: status}
	}
	huma.NewErrorWithContext = func(ctx huma.Context, status int, message string, errs ...error) huma.StatusError {
		if ctx.Operation().MaxBodyBytes != 0 {
			switch message {
			case "request body is required":
				return newAPIError(ctx.Context(), http.StatusBadRequest, "invalid_json", "EOF")
			case "validation failed", "cannot read request body", "request body read timeout":
				if len(errs) > 0 {
					err := errs[0]
					if detail, ok := err.(*huma.ErrorDetail); ok {
						message = detail.Message // ErrorDetail.Value 可能包含凭证。
					} else {
						message = err.Error()
					}
				}
				return newAPIError(ctx.Context(), http.StatusBadRequest, "invalid_json", message)
			}
			if status == http.StatusRequestEntityTooLarge {
				return newAPIError(ctx.Context(), http.StatusBadRequest, "invalid_json", "http: request body too large")
			}
		}
		return newAPIError(ctx.Context(), status, "internal_error", message)
	}
}

type jsonAPI struct{ huma.API }

func (a jsonAPI) Marshal(w io.Writer, _ string, value any) error {
	return json.NewEncoder(w).Encode(value)
}

func (a jsonAPI) Unmarshal(_ string, data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func (a jsonAPI) Negotiate(_ string) (string, error) {
	return "application/json; charset=utf-8", nil
}

// NewAPI 在现有路由上注册 Huma 接口。OpenAPI 由离线命令导出。
func NewAPI(mux humago.Mux) huma.API {
	config := huma.DefaultConfig("xa 接口文档", "1.0.0")
	config.OpenAPIPath = ""
	config.DocsPath = ""
	config.SchemasPath = ""
	config.CreateHooks = nil
	config.OpenAPI.Servers = []*huma.Server{{URL: "http://localhost:18080", Description: "本地 API 服务"}}
	config.OpenAPI.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		bearerScheme: {Type: "http", Scheme: "bearer"},
	}
	config.Transformers = []huma.Transformer{func(ctx huma.Context, _ string, value any) (any, error) {
		ctx.SetHeader("Cache-Control", "no-store")
		return value, nil
	}}
	return jsonAPI{API: humago.New(mux, config)}
}

// Operation 描述一个 JSON 接口及其实际可能返回的错误状态。
func Operation(api huma.API, tag, id, method, path, summary string, status int, maxBodyBytes int64, errors map[int]string) huma.Operation {
	schema := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[apiError](), true, "APIError")
	responses := make(map[string]*huma.Response, len(errors))
	for code, description := range errors {
		responses[strconv.Itoa(code)] = &huma.Response{
			Description: description,
			Content:     map[string]*huma.MediaType{"application/json": {Schema: schema}},
		}
	}
	return huma.Operation{
		OperationID: id, Method: method, Path: path, Summary: summary, DefaultStatus: status,
		Tags:      []string{tag},
		Responses: responses, MaxBodyBytes: maxBodyBytes, BodyReadTimeout: -1,
		SkipValidateBody: true, SkipValidateParams: true,
	}
}

// OperationAuth 在解析输入前运行现有身份认证，并为 OpenAPI 声明 Bearer 认证。
func OperationAuth(op huma.Operation, middleware func(http.Handler) http.Handler, optional bool) huma.Operation {
	op.Security = []map[string][]string{{bearerScheme: {}}}
	if optional {
		op.Security = append(op.Security, map[string][]string{})
	}
	op.Middlewares = append(op.Middlewares, func(ctx huma.Context, next func(huma.Context)) {
		r, w := humago.Unwrap(ctx)
		middleware(http.HandlerFunc(func(_ http.ResponseWriter, authenticated *http.Request) {
			next(huma.WithContext(ctx, authenticated.Context()))
		})).ServeHTTP(w, r)
	})
	return op
}

// WriteAPIError 让保留的 net/http 入口与 Huma 共用错误响应。
func WriteAPIError(w http.ResponseWriter, err huma.StatusError) {
	WriteJSON(w, err.GetStatus(), err)
}
