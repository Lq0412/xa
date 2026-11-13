package rag

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-ego/gse"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/infra/provider"
)

// SearchInput 的 collection_ids 只能缩小版本绑定范围，不能授予新的读取权限。
type SearchInput struct {
	Query         string   `json:"query"`
	TopK          int      `json:"top_k,omitempty"`
	CollectionIDs []string `json:"collection_ids,omitempty"`
}

// SearchHit 保存当次命中的内容和出处；source_id 在所属工具结果内标识来源。
type SearchHit struct {
	Chunk
	SourceID       string  `json:"source_id"`
	Title          string  `json:"title"`
	ContentSHA256  string  `json:"content_sha256"`
	ChunkerVersion string  `json:"chunker_version"`
	Score          float64 `json:"score"`
	VectorRank     int     `json:"vector_rank"`
	KeywordRank    int     `json:"keyword_rank"`
	Truncated      bool    `json:"truncated"`
}

// SearchResult 直接进入 tool_results.data，历史追溯不依赖仍然存在的原文或切片。
type SearchResult struct {
	Query          string      `json:"query"`
	AgentVersionID string      `json:"agent_version_id"`
	RetrievedAt    time.Time   `json:"retrieved_at"`
	Items          []SearchHit `json:"items"`
}

// Searcher 复用向量客户端；分词词典只在启动时加载，查询不修改词典。
type Searcher struct {
	service   *Service
	agents    definition.Service
	model     *provider.BailianModel
	segmenter gse.Segmenter
}

// NewSearcher 装配按 Agent 版本授权的检索服务，不依赖 PDF 解析环境。
func NewSearcher(service *Service, agents definition.Service, model *provider.BailianModel) (*Searcher, error) {
	seg, err := gse.NewEmbed("zh")
	if err != nil {
		return nil, err
	}
	return &Searcher{service: service, agents: agents, model: model, segmenter: seg}, nil
}

// Search 每次读取运行所固定的不可变版本，再取绑定与调用者当前权限的交集。
func (s *Searcher) Search(ctx context.Context, actor auth.Principal, agentID, versionID string, in SearchInput) (SearchResult, error) {
	in.Query = strings.TrimSpace(in.Query)
	result := SearchResult{Query: in.Query, AgentVersionID: versionID, RetrievedAt: time.Now().UTC(), Items: []SearchHit{}}
	if in.Query == "" || utf8.RuneCountInString(in.Query) > 200 || in.TopK < 0 || in.TopK > 20 {
		return result, fault("invalid_arguments", "查询须为 1–200 字，top_k 须为 1–20", 400)
	}
	if in.TopK == 0 {
		in.TopK = 5
	}
	agent, err := s.agents.Get(ctx, actor, agentID)
	if err != nil {
		return result, err
	}
	if agent.Status != definition.StatusActive {
		return result, definition.ErrNotFound
	}
	version, err := s.agents.GetVersion(ctx, actor, agentID, versionID)
	if err != nil {
		return result, err
	}
	requested := make(map[string]bool, len(in.CollectionIDs))
	for _, id := range in.CollectionIDs {
		requested[id] = true
	}
	ids := []string{}
	for _, b := range version.Configuration.Knowledge {
		if b.Enabled && len(b.Filter) == 0 && (len(requested) == 0 || requested[b.KnowledgeBaseID]) {
			ids = append(ids, b.KnowledgeBaseID)
		}
	}
	if len(ids) == 0 {
		return result, nil
	}
	// 无可读 ready 文档时直接返回空结果；最终查询仍重新检查权限和删除状态。
	var exists bool
	err = s.service.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM knowledge_bases b JOIN kb_documents d ON d.knowledge_base_id=b.id WHERE b.workspace_id=$1 AND (b.owner_user_id=$2 OR b.visibility='public') AND b.id=ANY($3) AND b.visibility=$4 AND b.deleted_at IS NULL AND d.status='ready')`, actor.WorkspaceID, actor.UserID, ids, agent.Visibility).Scan(&exists)
	if err != nil || !exists {
		return result, err
	}
	vectors, err := s.model.EmbedBatch(ctx, []string{in.Query}, "query")
	if err != nil {
		return result, &Fault{Code: "embedding_failed", Message: "查询向量生成失败，请稍后重试", Status: 502, cause: err}
	}
	vector, err := json.Marshal(vectors[0])
	if err != nil {
		return result, err
	}
	// 使用与摄取一致的 gse 搜索分词。OR 避免长问句中任一修饰词阻断专有名词召回。
	terms := strings.Fields(strings.Join(s.segmenter.CutSearch(in.Query, true), " "))
	query := strings.Join(terms, " OR ")
	tx, err := s.service.DB.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	// pgvector >= 0.8：权限过滤发生在近邻扫描中，迭代扫描补充被过滤的候选。
	if _, err = tx.Exec(ctx, `SET LOCAL hnsw.iterative_scan = strict_order`); err != nil {
		return result, err
	}
	cfg := s.service.Config
	rows, err := tx.Query(ctx, hybridSearchSQL, actor.WorkspaceID, actor.UserID, ids, provider.EmbeddingModel, agent.Visibility, string(vector), query, cfg.SearchCandidates, cfg.SearchMinSimilarity, cfg.RRFConstant, in.TopK)
	if err != nil {
		return result, err
	}
	remaining := cfg.SearchMaxChars
	for rows.Next() {
		var hit SearchHit
		err = rows.Scan(&hit.ID, &hit.DocumentID, &hit.KnowledgeBaseID, &hit.Index, &hit.Content, &hit.HeadingPath, &hit.PageFrom, &hit.PageTo, &hit.CharStart, &hit.CharEnd, &hit.TokenCount, &hit.EmbedModel, &hit.Title, &hit.ContentSHA256, &hit.ChunkerVersion, &hit.Score, &hit.VectorRank, &hit.KeywordRank)
		if err != nil {
			rows.Close()
			return result, err
		}
		if remaining == 0 {
			continue
		}
		hit.SourceID = hit.ID
		chars := []rune(hit.Content)
		if len(chars) > remaining {
			hit.Content = string(chars[:remaining])
			hit.CharEnd = hit.CharStart + remaining
			hit.TokenCount = tokenCount([]rune(hit.Content))
			hit.Truncated = true
		}
		remaining -= utf8.RuneCountInString(hit.Content)
		result.Items = append(result.Items, hit)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

// 两路候选均先约束权限、绑定、模型和 ready 状态，再应用 LIMIT，避免越权候选挤占名额。
const searchScopeSQL = ` b.workspace_id=$1 AND c.workspace_id=$1 AND d.workspace_id=$1
 AND (b.owner_user_id=$2 OR b.visibility='public') AND b.id=ANY($3)
 AND c.embed_model=$4 AND b.visibility=$5 AND b.deleted_at IS NULL AND d.status='ready' `

// RRF 使用排名而非混合不可比较的距离与关键词分数；并列时用切片 ID 保证结果稳定。
const hybridSearchSQL = `WITH vector_hits AS MATERIALIZED (
 SELECT c.id,c.embedding <=> $6::vector AS distance FROM kb_chunks c
 JOIN kb_documents d ON d.id=c.document_id JOIN knowledge_bases b ON b.id=c.knowledge_base_id
 WHERE ` + searchScopeSQL + ` ORDER BY c.embedding <=> $6::vector LIMIT $8
), keyword_hits AS MATERIALIZED (
 SELECT c.id,ts_rank_cd(c.fts,websearch_to_tsquery('simple',$7)) AS relevance FROM kb_chunks c
 JOIN kb_documents d ON d.id=c.document_id JOIN knowledge_bases b ON b.id=c.knowledge_base_id
 WHERE ` + searchScopeSQL + ` AND c.fts @@ websearch_to_tsquery('simple',$7)
 ORDER BY relevance DESC,c.id LIMIT $8
), v AS (
 SELECT id,row_number() OVER (ORDER BY distance,id) AS rank FROM vector_hits WHERE distance <= 1-$9::float8
), k AS (
 SELECT id,row_number() OVER (ORDER BY relevance DESC,id) AS rank FROM keyword_hits
), fused AS (
 SELECT coalesce(v.id,k.id) AS id,coalesce(1.0/($10::float8+v.rank),0)+coalesce(1.0/($10::float8+k.rank),0) AS score,
 coalesce(v.rank,0) AS vector_rank,coalesce(k.rank,0) AS keyword_rank FROM v FULL JOIN k ON v.id=k.id
)
SELECT c.id,c.document_id,c.knowledge_base_id,c.chunk_index,c.content,c.heading_path,c.page_from,c.page_to,
 c.char_start,c.char_end,c.token_count,c.embed_model,d.title,d.content_sha256,d.chunker_version,
 f.score,f.vector_rank,f.keyword_rank
FROM fused f JOIN kb_chunks c ON c.id=f.id JOIN kb_documents d ON d.id=c.document_id
ORDER BY f.score DESC,c.id LIMIT $11`
