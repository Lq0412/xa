-- R1 与记忆使用同一实例，文档、切片和任务保持独立。
CREATE TABLE knowledge_bases (
 id text PRIMARY KEY,
 workspace_id text NOT NULL REFERENCES workspaces(id),
 owner_user_id text NOT NULL REFERENCES users(id),
 visibility text NOT NULL CHECK (visibility IN ('private','public')),
 name text NOT NULL,
 description text NOT NULL DEFAULT '',
 embed_model text NOT NULL,
 chunker_version text NOT NULL,
 document_count integer NOT NULL DEFAULT 0 CHECK (document_count >= 0),
 chunk_count integer NOT NULL DEFAULT 0 CHECK (chunk_count >= 0),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 deleted_at timestamptz
);
CREATE UNIQUE INDEX knowledge_bases_owner_name ON knowledge_bases(owner_user_id,name) WHERE deleted_at IS NULL;
CREATE INDEX knowledge_bases_visible ON knowledge_bases(workspace_id,owner_user_id,visibility) WHERE deleted_at IS NULL;

CREATE TABLE kb_documents (
 id text PRIMARY KEY,
 workspace_id text NOT NULL REFERENCES workspaces(id),
 knowledge_base_id text NOT NULL REFERENCES knowledge_bases(id),
 owner_user_id text NOT NULL REFERENCES users(id),
 title text NOT NULL,
 filename text NOT NULL,
 mime text NOT NULL,
 size_bytes bigint NOT NULL CHECK (size_bytes > 0),
 content_sha256 text NOT NULL,
 object_key text NOT NULL UNIQUE,
 status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','ready','failed','deleting')),
 error_code text NOT NULL DEFAULT '',
 error_message text NOT NULL DEFAULT '',
 page_count integer NOT NULL DEFAULT 0,
 chunk_count integer NOT NULL DEFAULT 0,
 token_count integer NOT NULL DEFAULT 0,
 chunker_version text NOT NULL,
 embed_model text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 ready_at timestamptz
);
CREATE UNIQUE INDEX kb_documents_content ON kb_documents(knowledge_base_id,content_sha256) WHERE status <> 'deleting';
CREATE INDEX kb_documents_owner ON kb_documents(owner_user_id);

CREATE TABLE kb_chunks (
 id text PRIMARY KEY,
 workspace_id text NOT NULL REFERENCES workspaces(id),
 document_id text NOT NULL REFERENCES kb_documents(id) ON DELETE CASCADE,
 knowledge_base_id text NOT NULL REFERENCES knowledge_bases(id),
 chunk_index integer NOT NULL,
 content text NOT NULL,
 search_text text NOT NULL,
 fts tsvector GENERATED ALWAYS AS (to_tsvector('simple', search_text)) STORED,
 embedding vector(1024) NOT NULL,
 heading_path text[] NOT NULL DEFAULT '{}',
 page_from integer,
 page_to integer,
 char_start integer NOT NULL,
 char_end integer NOT NULL,
 token_count integer NOT NULL,
 embed_model text NOT NULL,
 UNIQUE(document_id,chunk_index)
);
CREATE INDEX kb_chunks_fts ON kb_chunks USING gin(fts);
CREATE INDEX kb_chunks_embedding ON kb_chunks USING hnsw(embedding vector_cosine_ops);
CREATE INDEX kb_chunks_base ON kb_chunks(knowledge_base_id);
-- 本机 pgvector 0.8.7；R2 带权限过滤的查询应在事务内打开 hnsw.iterative_scan。

CREATE TABLE kb_ingest_jobs (
 id text PRIMARY KEY,
 workspace_id text NOT NULL REFERENCES workspaces(id),
 document_id text NOT NULL,
 kind text NOT NULL CHECK (kind IN ('ingest','delete','rebuild')),
 owner_user_id text NOT NULL REFERENCES users(id),
 status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','completed','failed','dead')),
 attempts integer NOT NULL DEFAULT 0,
 next_run_at timestamptz NOT NULL DEFAULT now(),
 last_error text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 completed_at timestamptz
);
-- 删除文档后保留任务完成证据，因此 document_id 不设外键。
CREATE UNIQUE INDEX kb_ingest_jobs_active ON kb_ingest_jobs(kind,document_id) WHERE status IN ('pending','running');
CREATE INDEX kb_ingest_jobs_due ON kb_ingest_jobs(next_run_at,created_at) WHERE status='pending';
