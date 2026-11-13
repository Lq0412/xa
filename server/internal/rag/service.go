package rag

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/framework/config"
	"linq-bs/server/internal/framework/storage"
	"linq-bs/server/internal/infra/provider"
)

// Base 是单一所有者的知识库，公共库也保留明确所有者用于配额统计。
type Base struct {
	ID             string    `json:"id"`
	WorkspaceID    string    `json:"workspace_id"`
	OwnerUserID    string    `json:"owner_user_id"`
	Visibility     string    `json:"visibility" enum:"private,public"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	EmbedModel     string    `json:"embed_model"`
	ChunkerVersion string    `json:"chunker_version"`
	DocumentCount  int       `json:"document_count"`
	ChunkCount     int       `json:"chunk_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Document 暴露摄取状态和可解释错误，原始对象键仅在服务内部使用。
type Document struct {
	ID              string     `json:"id"`
	WorkspaceID     string     `json:"workspace_id"`
	KnowledgeBaseID string     `json:"knowledge_base_id"`
	OwnerUserID     string     `json:"owner_user_id"`
	Title           string     `json:"title"`
	Filename        string     `json:"filename"`
	MIME            string     `json:"mime"`
	SizeBytes       int64      `json:"size_bytes"`
	ContentSHA256   string     `json:"content_sha256"`
	ObjectKey       string     `json:"-"`
	Status          string     `json:"status" enum:"pending,processing,ready,failed,deleting"`
	ErrorCode       string     `json:"error_code" doc:"摄取失败码：unsupported_file_type、invalid_file_type、invalid_utf8、file_too_large、empty_document、too_many_pages、too_many_characters、scanned_pdf、pdf_parse_failed、source_unavailable、embedding_failed、ingest_failed、delete_failed"`
	ErrorMessage    string     `json:"error_message"`
	PageCount       int        `json:"page_count"`
	ChunkCount      int        `json:"chunk_count"`
	TokenCount      int        `json:"token_count"`
	ChunkerVersion  string     `json:"chunker_version"`
	EmbedModel      string     `json:"embed_model"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	ReadyAt         *time.Time `json:"ready_at"`
}

// BasePage、DocumentPage、ChunkPage 提供稳定排序的分页结果。
type BasePage struct {
	Items []Base `json:"items"`
	Total int    `json:"total"`
}
type DocumentPage struct {
	Items []Document `json:"items"`
	Total int        `json:"total"`
}
type ChunkPage struct {
	Items []Chunk `json:"items"`
	Total int     `json:"total"`
}

// CreateInput 创建后不再修改 visibility 或所有权。
type CreateInput struct {
	Name        string `json:"name" minLength:"1" maxLength:"100"`
	Description string `json:"description,omitempty" maxLength:"2000"`
	Visibility  string `json:"visibility,omitempty" enum:"private,public" default:"private"`
}

// UpdateInput 只更新显式提交的名称和描述。
type UpdateInput struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// Service 使用与记忆相同的基础设施，但保持独立数据和任务边界。
type Service struct {
	DB      *pgxpool.Pool
	Objects storage.ObjectStore
	Config  config.KnowledgeConfig
}

type querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

const baseColumns = `id,workspace_id,owner_user_id,visibility,name,description,embed_model,chunker_version,document_count,chunk_count,created_at,updated_at`
const documentColumns = `id,workspace_id,knowledge_base_id,owner_user_id,title,filename,mime,size_bytes,content_sha256,object_key,status,error_code,error_message,page_count,chunk_count,token_count,chunker_version,embed_model,created_at,updated_at,ready_at`

func scanBase(row pgx.Row) (Base, error) {
	var b Base
	err := row.Scan(&b.ID, &b.WorkspaceID, &b.OwnerUserID, &b.Visibility, &b.Name, &b.Description, &b.EmbedModel, &b.ChunkerVersion, &b.DocumentCount, &b.ChunkCount, &b.CreatedAt, &b.UpdatedAt)
	return b, dbError(err)
}
func scanDocument(row pgx.Row) (Document, error) {
	var d Document
	err := row.Scan(&d.ID, &d.WorkspaceID, &d.KnowledgeBaseID, &d.OwnerUserID, &d.Title, &d.Filename, &d.MIME, &d.SizeBytes, &d.ContentSHA256, &d.ObjectKey, &d.Status, &d.ErrorCode, &d.ErrorMessage, &d.PageCount, &d.ChunkCount, &d.TokenCount, &d.ChunkerVersion, &d.EmbedModel, &d.CreatedAt, &d.UpdatedAt, &d.ReadyAt)
	return d, dbError(err)
}
func dbError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fault("not_found", "知识库或文档不存在", 404)
	}
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) && pgerr.Code == "23505" {
		return fault("name_conflict", "已存在同名知识库", 409)
	}
	return err
}

func getBase(ctx context.Context, q querier, p auth.Principal, id string, write, lock bool) (Base, error) {
	query := `SELECT ` + baseColumns + ` FROM knowledge_bases WHERE id=$1 AND workspace_id=$2 AND deleted_at IS NULL AND (owner_user_id=$3 OR visibility='public')`
	if lock {
		query += ` FOR UPDATE`
	}
	b, err := scanBase(q.QueryRow(ctx, query, id, p.WorkspaceID, p.UserID))
	if err != nil {
		return b, err
	}
	if write && b.Visibility == "public" && !auth.HasRole(p, "admin") {
		return Base{}, fault("forbidden", "只有管理员可以维护公共知识库", 403)
	}
	return b, nil
}

// Get 以当前调用者权限读取，不给管理员增加查看他人私有库的权限。
func (s *Service) Get(ctx context.Context, p auth.Principal, id string) (Base, error) {
	return getBase(ctx, s.DB, p, id, false, false)
}

// List 包含自己的未删除库和同 workspace 的公共库。
func (s *Service) List(ctx context.Context, p auth.Principal, limit, offset int) (BasePage, error) {
	result := BasePage{Items: []Base{}}
	rows, err := s.DB.Query(ctx, `SELECT `+baseColumns+` FROM knowledge_bases WHERE workspace_id=$1 AND deleted_at IS NULL AND (owner_user_id=$2 OR visibility='public') ORDER BY created_at DESC,id LIMIT $3 OFFSET $4`, p.WorkspaceID, p.UserID, limit, offset)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		b, err := scanBase(rows)
		if err != nil {
			return result, err
		}
		result.Items = append(result.Items, b)
	}
	if err = rows.Err(); err != nil {
		return result, err
	}
	rows.Close()
	err = s.DB.QueryRow(ctx, `SELECT count(*) FROM knowledge_bases WHERE workspace_id=$1 AND deleted_at IS NULL AND (owner_user_id=$2 OR visibility='public')`, p.WorkspaceID, p.UserID).Scan(&result.Total)
	return result, err
}

func validMetadata(name, description string) error {
	if strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 100 || utf8.RuneCountInString(description) > 2000 {
		return fault("invalid_knowledge_base", "名称须为 1–100 字，描述最多 2000 字", 400)
	}
	return nil
}

// Create 锁定所有者行后统计配额，避免并发创建突破限制。
func (s *Service) Create(ctx context.Context, p auth.Principal, in CreateInput) (Base, error) {
	in.Name = strings.TrimSpace(in.Name)
	if err := validMetadata(in.Name, in.Description); err != nil {
		return Base{}, err
	}
	if in.Visibility == "" {
		in.Visibility = "private"
	}
	if in.Visibility != "private" && in.Visibility != "public" {
		return Base{}, fault("invalid_knowledge_base", "知识库可见性无效", 400)
	}
	if in.Visibility == "public" && !auth.HasRole(p, "admin") {
		return Base{}, fault("forbidden", "只有管理员可以创建公共知识库", 403)
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Base{}, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	if _, err = tx.Exec(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, p.UserID); err != nil {
		return Base{}, err
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM knowledge_bases WHERE owner_user_id=$1 AND deleted_at IS NULL`, p.UserID).Scan(&count); err != nil {
		return Base{}, err
	}
	if count >= s.Config.MaxBasesPerUser {
		return Base{}, fault("knowledge_base_quota", "知识库数量已达到上限", 409)
	}
	b, err := scanBase(tx.QueryRow(ctx, `INSERT INTO knowledge_bases(id,workspace_id,owner_user_id,visibility,name,description,embed_model,chunker_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING `+baseColumns, "kb_"+uuid.NewString(), p.WorkspaceID, p.UserID, in.Visibility, in.Name, in.Description, provider.EmbeddingModel, ChunkerVersion(s.Config)))
	if err != nil {
		return b, err
	}
	return b, tx.Commit(ctx)
}

// Update 在行锁下保留未提交的字段，避免并发部分更新覆盖其他字段。
func (s *Service) Update(ctx context.Context, p auth.Principal, id string, in UpdateInput) (Base, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Base{}, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	b, err := getBase(ctx, tx, p, id, true, true)
	if err != nil {
		return b, err
	}
	if in.Name != nil {
		b.Name = strings.TrimSpace(*in.Name)
	}
	if in.Description != nil {
		b.Description = *in.Description
	}
	if err = validMetadata(b.Name, b.Description); err != nil {
		return Base{}, err
	}
	b, err = scanBase(tx.QueryRow(ctx, `UPDATE knowledge_bases SET name=$2,description=$3,updated_at=now() WHERE id=$1 RETURNING `+baseColumns, id, b.Name, b.Description))
	if err != nil {
		return b, err
	}
	return b, tx.Commit(ctx)
}

// Upload 先存原文，再以同一事务提交文档与任务；重复上传不创建第二份文档。
func (s *Service) Upload(ctx context.Context, p auth.Principal, id, filename string, data []byte) (doc Document, duplicate bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, s.Config.JobTimeout)
	defer cancel()
	b, err := getBase(ctx, s.DB, p, id, true, false)
	if err != nil {
		return doc, false, err
	}
	mime, err := ValidateFile(filename, data, s.Config)
	if err != nil {
		return doc, false, err
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	doc, err = scanDocument(s.DB.QueryRow(ctx, `SELECT `+documentColumns+` FROM kb_documents WHERE knowledge_base_id=$1 AND content_sha256=$2 AND status<>'deleting'`, id, hash))
	if err == nil {
		return doc, true, nil
	}
	var missing *Fault
	if !errors.As(err, &missing) || missing.Code != "not_found" {
		return doc, false, err
	}
	key := s.objectPrefix() + b.WorkspaceID + "/" + uuid.NewString()
	if err = s.Objects.Put(ctx, key, bytes.NewReader(data), int64(len(data)), mime); err != nil {
		return doc, false, err
	}
	persisted := false
	defer func() {
		if persisted {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if cleanupErr := s.Objects.Delete(cleanupCtx, key); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("清理未入库原文失败: %w", cleanupErr))
		}
	}()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return doc, false, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	// 所有管理员上传公共库时，配额统一计入知识库所有者；先用户、后知识库锁。
	if _, err = tx.Exec(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, b.OwnerUserID); err != nil {
		return doc, false, err
	}
	b, err = getBase(ctx, tx, p, id, true, true)
	if err != nil {
		return doc, false, err
	}
	existing, e := scanDocument(tx.QueryRow(ctx, `SELECT `+documentColumns+` FROM kb_documents WHERE knowledge_base_id=$1 AND content_sha256=$2 AND status<>'deleting'`, id, hash))
	if e == nil {
		return existing, true, nil
	}
	if !errors.As(e, &missing) || missing.Code != "not_found" {
		return doc, false, e
	}
	if b.DocumentCount >= s.Config.MaxDocuments {
		return doc, false, fault("document_quota", "知识库文档数量已达到上限", 409)
	}
	var used int64
	// deleting 原文在物理删除前仍占空间，防止反复上传/删除越过存储配额。
	if err = tx.QueryRow(ctx, `SELECT coalesce(sum(size_bytes),0) FROM kb_documents WHERE owner_user_id=$1`, b.OwnerUserID).Scan(&used); err != nil {
		return doc, false, err
	}
	if used+int64(len(data)) > s.Config.MaxUserBytes {
		return doc, false, fault("storage_quota", "知识库原文总大小已达到用户配额", 409)
	}
	filename = filepath.Base(filename)
	doc, err = scanDocument(tx.QueryRow(ctx, `INSERT INTO kb_documents(id,workspace_id,knowledge_base_id,owner_user_id,title,filename,mime,size_bytes,content_sha256,object_key,chunker_version,embed_model) VALUES($1,$2,$3,$4,$5,$5,$6,$7,$8,$9,$10,$11) RETURNING `+documentColumns, "doc_"+uuid.NewString(), b.WorkspaceID, b.ID, b.OwnerUserID, filename, mime, len(data), hash, key, ChunkerVersion(s.Config), provider.EmbeddingModel))
	if err != nil {
		return doc, false, err
	}
	if err = enqueue(ctx, tx, doc, "ingest"); err != nil {
		return doc, false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE knowledge_bases SET document_count=document_count+1,updated_at=now() WHERE id=$1`, id); err != nil {
		return doc, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return doc, false, err
	}
	persisted = true
	return doc, false, nil
}

func enqueue(ctx context.Context, tx pgx.Tx, d Document, kind string) error {
	_, err := tx.Exec(ctx, `INSERT INTO kb_ingest_jobs(id,workspace_id,document_id,kind,owner_user_id) VALUES($1,$2,$3,$4,$5) ON CONFLICT(kind,document_id) WHERE status IN ('pending','running') DO NOTHING`, "kbjob_"+uuid.NewString(), d.WorkspaceID, d.ID, kind, d.OwnerUserID)
	return err
}

// 不同数据库的对象使用独立前缀，避免验收数据库的孤儿清理触及日常数据。
func (s *Service) objectPrefix() string {
	return "knowledge/" + s.DB.Config().ConnConfig.Database + "/"
}

// GetDocument 同时约束所属知识库，隐藏 deleting 文档。
func (s *Service) GetDocument(ctx context.Context, p auth.Principal, baseID, id string) (Document, error) {
	if _, err := s.Get(ctx, p, baseID); err != nil {
		return Document{}, err
	}
	return scanDocument(s.DB.QueryRow(ctx, `SELECT `+documentColumns+` FROM kb_documents WHERE id=$1 AND knowledge_base_id=$2 AND status<>'deleting'`, id, baseID))
}

// Documents 返回可见文档及失败原因。
func (s *Service) Documents(ctx context.Context, p auth.Principal, id string, limit, offset int) (DocumentPage, error) {
	result := DocumentPage{Items: []Document{}}
	if _, err := s.Get(ctx, p, id); err != nil {
		return result, err
	}
	rows, err := s.DB.Query(ctx, `SELECT `+documentColumns+` FROM kb_documents WHERE knowledge_base_id=$1 AND status<>'deleting' ORDER BY created_at DESC,id LIMIT $2 OFFSET $3`, id, limit, offset)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return result, err
		}
		result.Items = append(result.Items, d)
	}
	if err = rows.Err(); err != nil {
		return result, err
	}
	rows.Close()
	err = s.DB.QueryRow(ctx, `SELECT count(*) FROM kb_documents WHERE knowledge_base_id=$1 AND status<>'deleting'`, id).Scan(&result.Total)
	return result, err
}

// Chunks 分页读取 ready 文档切片，字段足够支持 R2 的引用快照。
func (s *Service) Chunks(ctx context.Context, p auth.Principal, baseID, id string, limit, offset int) (ChunkPage, error) {
	result := ChunkPage{Items: []Chunk{}}
	d, err := s.GetDocument(ctx, p, baseID, id)
	if err != nil {
		return result, err
	}
	if d.Status != "ready" {
		return result, nil
	}
	result.Total = d.ChunkCount
	rows, err := s.DB.Query(ctx, `SELECT c.id,c.document_id,c.knowledge_base_id,c.chunk_index,c.content,c.heading_path,c.page_from,c.page_to,c.char_start,c.char_end,c.token_count,c.embed_model FROM kb_chunks c JOIN kb_documents d ON d.id=c.document_id JOIN knowledge_bases b ON b.id=c.knowledge_base_id WHERE c.document_id=$1 AND d.status='ready' AND b.deleted_at IS NULL ORDER BY c.chunk_index LIMIT $2 OFFSET $3`, id, limit, offset)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var c Chunk
		if err = rows.Scan(&c.ID, &c.DocumentID, &c.KnowledgeBaseID, &c.Index, &c.Content, &c.HeadingPath, &c.PageFrom, &c.PageTo, &c.CharStart, &c.CharEnd, &c.TokenCount, &c.EmbedModel); err != nil {
			return result, err
		}
		result.Items = append(result.Items, c)
	}
	return result, rows.Err()
}

// Delete 先隐藏知识库和文档，再排队逐份删除原文与切片。
func (s *Service) Delete(ctx context.Context, p auth.Principal, id string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	if _, err = getBase(ctx, tx, p, id, true, true); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `UPDATE kb_documents SET status='deleting',updated_at=now() WHERE knowledge_base_id=$1 AND status<>'deleting' RETURNING `+documentColumns, id)
	if err != nil {
		return err
	}
	docs := []Document{}
	for rows.Next() {
		d, e := scanDocument(rows)
		if e != nil {
			rows.Close()
			return e
		}
		docs = append(docs, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, d := range docs {
		if err = enqueue(ctx, tx, d, "delete"); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE knowledge_bases SET deleted_at=now(),updated_at=now(),document_count=0,chunk_count=0 WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// DeleteDocument 的计数与软删除同事务完成；工作进程不再发布它的摄取结果。
func (s *Service) DeleteDocument(ctx context.Context, p auth.Principal, baseID, id string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	if _, err = getBase(ctx, tx, p, baseID, true, true); err != nil {
		return err
	}
	d, err := scanDocument(tx.QueryRow(ctx, `UPDATE kb_documents SET status='deleting',updated_at=now() WHERE id=$1 AND knowledge_base_id=$2 AND status<>'deleting' RETURNING `+documentColumns, id, baseID))
	if err != nil {
		return err
	}
	if err = enqueue(ctx, tx, d, "delete"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE knowledge_bases SET document_count=document_count-1,chunk_count=chunk_count-$2,updated_at=now() WHERE id=$1`, baseID, d.ChunkCount); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Retry 只接受 failed；与正在执行的任务串行，不插入第二个活动任务。
func (s *Service) Retry(ctx context.Context, p auth.Principal, baseID, id string) (Document, error) {
	if _, err := getBase(ctx, s.DB, p, baseID, true, false); err != nil {
		return Document{}, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Document{}, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	// 与 worker 一样先任务锁、再知识库和文档锁，避免手动重试与自动重试相互等待。
	if _, err = tx.Exec(ctx, `SELECT id FROM kb_ingest_jobs WHERE document_id=$1 AND kind IN ('ingest','rebuild') AND status IN ('pending','running') FOR UPDATE`, id); err != nil {
		return Document{}, err
	}
	if _, err = getBase(ctx, tx, p, baseID, true, true); err != nil {
		return Document{}, err
	}
	d, err := scanDocument(tx.QueryRow(ctx, `SELECT `+documentColumns+` FROM kb_documents WHERE id=$1 AND knowledge_base_id=$2 AND status<>'deleting' FOR UPDATE`, id, baseID))
	if err != nil {
		return d, err
	}
	if d.Status != "failed" {
		return d, fault("document_not_failed", "只有失败文档可以重试", 409)
	}
	if _, err = tx.Exec(ctx, `UPDATE kb_ingest_jobs SET status='failed',completed_at=now() WHERE document_id=$1 AND kind IN ('ingest','rebuild') AND status='pending'`, id); err != nil {
		return d, err
	}
	d, err = scanDocument(tx.QueryRow(ctx, `UPDATE kb_documents SET status='pending',error_code='',error_message='',updated_at=now() WHERE id=$1 RETURNING `+documentColumns, id))
	if err != nil {
		return d, err
	}
	if err = enqueue(ctx, tx, d, "ingest"); err != nil {
		return d, err
	}
	return d, tx.Commit(ctx)
}
