ALTER TABLE messages ADD COLUMN IF NOT EXISTS parts jsonb;
CREATE INDEX IF NOT EXISTS runs_ended_retention ON runs(ended_at, id) WHERE ended_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS run_events_created_idx ON run_events(created_at);
CREATE INDEX IF NOT EXISTS run_contexts_created_idx ON run_contexts(created_at);
CREATE INDEX IF NOT EXISTS run_spans_started_idx ON run_spans(started_at);
CREATE INDEX IF NOT EXISTS tool_results_created_idx ON tool_results(created_at);
