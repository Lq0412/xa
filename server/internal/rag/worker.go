package rag

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/go-ego/gse"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"linq-bs/server/internal/infra/provider"
)

// Ingestor 在现有 worker 内处理独立队列；分词器只在启动时载入内置中文词典。
type Ingestor struct {
	Service   *Service
	Model     *provider.BailianModel
	segmenter gse.Segmenter
}

// NewIngestor 启动时检查 Poppler，缺失依赖直接报告，不在任务中静默跳过 PDF。
func NewIngestor(s *Service, model *provider.BailianModel) (*Ingestor, error) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		return nil, errors.New("知识摄取需要 poppler-utils，请先安装 pdftotext")
	}
	segmenter, err := gse.NewEmbed("zh")
	if err != nil {
		return nil, err
	}
	return &Ingestor{Service: s, Model: model, segmenter: segmenter}, nil
}

// Run 逐条执行任务，并按配置周期清理已超过宽限期且没有文档引用的对象。
func (w *Ingestor) Run(ctx context.Context, logger *slog.Logger) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	cleanup := time.NewTicker(w.Service.Config.CleanupInterval)
	defer cleanup.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := w.ProcessNext(ctx); err != nil && ctx.Err() == nil {
				logger.Error("知识摄取任务失败", "error", err)
			}
		case <-cleanup.C:
			if err := w.Service.CleanupOrphans(ctx); err != nil && ctx.Err() == nil {
				logger.Error("知识原文清理失败", "error", err)
			}
		}
	}
}

// CleanupOrphans 的宽限期长于任务超时，正常上传不会在事务提交前被清理。
func (s *Service) CleanupOrphans(ctx context.Context) error {
	return s.Objects.ListBefore(ctx, s.objectPrefix(), time.Now().Add(-s.Config.OrphanGrace), func(key string) error {
		var exists bool
		if err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM kb_documents WHERE object_key=$1)`, key).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return nil
		}
		return s.Objects.Delete(ctx, key)
	})
}

// ProcessNext 用 SKIP LOCKED 领取任务并持锁到完成；退出回滚后，任务仍可被下一进程领取。
func (w *Ingestor) ProcessNext(ctx context.Context) (bool, error) {
	s := w.Service
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	var jobID, docID, kind string
	err = tx.QueryRow(ctx, `SELECT id,document_id,kind FROM kb_ingest_jobs WHERE status='pending' AND next_run_at<=now() ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&jobID, &docID, &kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE kb_ingest_jobs SET status='running' WHERE id=$1`, jobID); err != nil {
		return true, err
	}
	workCtx, cancel := context.WithTimeout(ctx, s.Config.JobTimeout)
	var chunks []Chunk
	var parsed Parsed
	var d Document
	// processing 单独提交，用户能观察到正在处理；任务行锁仍在外层事务中，崩溃可重新领取。
	if kind == "ingest" || kind == "rebuild" {
		d, err = scanDocument(s.DB.QueryRow(workCtx, `UPDATE kb_documents SET status='processing',error_code='',error_message='',updated_at=now() WHERE id=$1 AND status<>'deleting' RETURNING `+documentColumns, docID))
		if err == nil {
			parsed, chunks, err = w.prepare(workCtx, d)
		}
	} else {
		d, err = scanDocument(s.DB.QueryRow(workCtx, `SELECT `+documentColumns+` FROM kb_documents WHERE id=$1 AND status='deleting'`, docID))
		if err == nil {
			err = s.Objects.Delete(workCtx, d.ObjectKey)
		}
	}
	cancel()
	var f *Fault
	if errors.As(err, &f) && f.Code == "not_found" {
		err = nil
	}
	if err != nil {
		failure := &Fault{Code: "ingest_failed", Message: "摄取失败，请稍后重试", Status: 500}
		if kind == "delete" {
			failure = &Fault{Code: "delete_failed", Message: "原文清理失败，后台将重试", Status: 500}
		}
		if errors.As(err, &f) {
			failure = f
		}
		if _, e := tx.Exec(ctx, `UPDATE kb_documents SET status=CASE WHEN status='deleting' THEN status ELSE 'failed' END,error_code=$2,error_message=$3,page_count=CASE WHEN $4>0 THEN $4 ELSE page_count END,updated_at=now() WHERE id=$1`, docID, failure.Code, failure.Message, parsed.PageCount); e != nil {
			return true, e
		}
		if _, e := tx.Exec(ctx, `UPDATE kb_ingest_jobs SET attempts=attempts+1,last_error=$2,status=CASE WHEN attempts+1>=3 THEN 'dead' ELSE 'pending' END,next_run_at=now()+CASE attempts WHEN 0 THEN interval '30 seconds' WHEN 1 THEN interval '2 minutes' ELSE interval '10 minutes' END,completed_at=CASE WHEN attempts+1>=3 THEN now() END WHERE id=$1`, jobID, failure.Message); e != nil {
			return true, e
		}
		if e := tx.Commit(ctx); e != nil {
			return true, e
		}
		return true, failure
	}
	if d.ID != "" {
		// 与删除接口采用相同的锁顺序；正在删除的文档不能被摄取重新置为 ready。
		if _, err = tx.Exec(ctx, `SELECT id FROM knowledge_bases WHERE id=$1 FOR UPDATE`, d.KnowledgeBaseID); err != nil {
			return true, err
		}
		var status string
		err = tx.QueryRow(ctx, `SELECT status FROM kb_documents WHERE id=$1 FOR UPDATE`, d.ID).Scan(&status)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return true, err
		}
		if err == nil && kind == "delete" && status == "deleting" {
			if _, err = tx.Exec(ctx, `DELETE FROM kb_documents WHERE id=$1`, d.ID); err != nil {
				return true, err
			}
		} else if err == nil && kind != "delete" && status != "deleting" {
			if err = w.publish(ctx, tx, d, parsed, chunks); err != nil {
				return true, err
			}
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE kb_ingest_jobs SET status='completed',completed_at=now(),last_error='' WHERE id=$1`, jobID); err != nil {
		return true, err
	}
	return true, tx.Commit(ctx)
}

func (w *Ingestor) prepare(ctx context.Context, d Document) (Parsed, []Chunk, error) {
	s := w.Service
	object, err := s.Objects.Open(ctx, d.ObjectKey)
	if err != nil {
		return Parsed{}, nil, fault("source_unavailable", "无法读取原文，请稍后重试", 500)
	}
	data, err := io.ReadAll(io.LimitReader(object, s.Config.MaxFileBytes+1))
	closeErr := object.Close()
	if err != nil || closeErr != nil {
		return Parsed{}, nil, fault("source_unavailable", "读取原文失败，请稍后重试", 500)
	}
	if _, err = ValidateFile(d.Filename, data, s.Config); err != nil {
		return Parsed{}, nil, err
	}
	parsed, err := Parse(ctx, data, d.MIME, s.Config)
	if err != nil {
		return parsed, nil, err
	}
	chunks := Split(parsed, s.Config)
	for i := range chunks {
		chunks[i].SearchText = strings.Join(strings.Fields(strings.Join(w.segmenter.CutSearch(chunks[i].Content, true), " ")), " ")
		chunks[i].EmbedModel = provider.EmbeddingModel
	}
	for start := 0; start < len(chunks); start += s.Config.EmbeddingBatchSize {
		end := min(start+s.Config.EmbeddingBatchSize, len(chunks))
		inputs := make([]string, end-start)
		for i := start; i < end; i++ {
			inputs[i-start] = chunks[i].Content
		}
		vectors, e := w.Model.EmbedBatch(ctx, inputs, s.Config.TextType)
		if e != nil {
			return parsed, nil, &Fault{Code: "embedding_failed", Message: "向量化失败，请稍后重试", Status: 502, cause: e}
		}
		for i := start; i < end; i++ {
			chunks[i].Embedding = vectors[i-start]
		}
	}
	return parsed, chunks, nil
}

// publish 与任务完成共用事务：先清除全部旧切片，再替换并更新计数。
func (w *Ingestor) publish(ctx context.Context, tx pgx.Tx, d Document, parsed Parsed, chunks []Chunk) error {
	if _, err := tx.Exec(ctx, `DELETE FROM kb_chunks WHERE document_id=$1`, d.ID); err != nil {
		return err
	}
	tokens := 0
	for _, c := range chunks {
		vector, err := json.Marshal(c.Embedding)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO kb_chunks(id,workspace_id,document_id,knowledge_base_id,chunk_index,content,search_text,embedding,heading_path,page_from,page_to,char_start,char_end,token_count,embed_model) VALUES($1,$2,$3,$4,$5,$6,$7,$8::vector,$9,$10,$11,$12,$13,$14,$15)`, "chunk_"+uuid.NewString(), d.WorkspaceID, d.ID, d.KnowledgeBaseID, c.Index, c.Content, c.SearchText, string(vector), c.HeadingPath, c.PageFrom, c.PageTo, c.CharStart, c.CharEnd, c.TokenCount, c.EmbedModel); err != nil {
			return err
		}
		tokens += c.TokenCount
	}
	if _, err := tx.Exec(ctx, `UPDATE kb_documents SET status='ready',error_code='',error_message='',page_count=$2,chunk_count=$3,token_count=$4,chunker_version=$5,embed_model=$6,ready_at=now(),updated_at=now() WHERE id=$1`, d.ID, parsed.PageCount, len(chunks), tokens, ChunkerVersion(w.Service.Config), provider.EmbeddingModel); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE knowledge_bases SET chunk_count=chunk_count+$2-$3,updated_at=now() WHERE id=$1`, d.KnowledgeBaseID, len(chunks), d.ChunkCount)
	return err
}
