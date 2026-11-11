import { ArrowLeft, ChevronDown, ChevronRight, RefreshCw } from 'lucide-react';
import { useCallback, useState } from 'react';
import { Link, useParams, useSearchParams } from 'umi';
import { ErrorNotice, Page } from '@/components/Page';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { useResource } from '@/lib/useResource';
import { getRun, type RunDetail, type SpanNode } from '@/services/xa/runs';
import { dateTime, duration, RunStatus } from './format';

const kinds: Record<string, string> = {
  run: '运行',
  context: '上下文',
  context_segment: '上下文段',
  llm: '模型',
  tool: '工具',
  approval: '审批',
  memory_recall: '记忆召回',
  memory_write: '记忆写入',
  subrun: '子运行',
};

/** 运行中的 span 仍使用 ok；是否结束必须以 ended_at 判定。 */
function spanStatus(node: SpanNode) {
  return node.ended_at === null
    ? node.kind === 'approval'
      ? 'waiting_approval'
      : 'running'
    : node.status;
}

function tokenCount(value: number | null) {
  return value === null ? '—' : value.toLocaleString('zh-CN');
}

function findNode(node: SpanNode, id: string): SpanNode | undefined {
  if (node.id === id) return node;
  for (const child of node.children) {
    const found = findNode(child, id);
    if (found) return found;
  }
  return undefined;
}

/** 使用原生按钮实现点选与折叠，键盘可逐项到达，不依赖指针操作。 */
function SpanBranch({
  node,
  selected,
  onSelect,
}: {
  node: SpanNode;
  selected: string;
  onSelect: (id: string) => void;
}) {
  const [expanded, setExpanded] = useState(
    (node.kind !== 'context' && node.kind !== 'context_segment') ||
      (node.id !== selected && !!findNode(node, selected)),
  );
  const hasChildren = node.children.length > 0;
  const tokens = (node.prompt_tokens ?? 0) + (node.completion_tokens ?? 0);
  const status = spanStatus(node);
  return (
    <li>
      <div
        className={`span-row ${selected === node.id ? 'span-selected' : ''}`}
      >
        {hasChildren ? (
          <button
            type="button"
            className="span-toggle"
            aria-label={`${expanded ? '收起' : '展开'}${node.name}`}
            aria-expanded={expanded}
            aria-controls={`children-${node.id}`}
            onClick={() => setExpanded(!expanded)}
          >
            {expanded ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
          </button>
        ) : (
          <span className="w-6 shrink-0" />
        )}
        <button
          type="button"
          className="span-select"
          aria-pressed={selected === node.id}
          onClick={() => onSelect(node.id)}
        >
          <span className="min-w-0 flex-1 text-left">
            <span className="block truncate" title={node.name}>
              {node.kind === 'run' ? '运行' : node.name}
            </span>
            <span className="text-[10px] text-zinc-400">
              {kinds[node.kind] || node.kind}
              {node.step !== null && node.step > 0 && ` · 第 ${node.step} 步`}
            </span>
          </span>
          {status !== 'ok' && (
            <span className="text-[11px]">
              <RunStatus value={status} />
            </span>
          )}
          <span className="span-measure">
            <span>{duration(node.duration_ms)}</span>
            {tokens > 0 && <span>{tokens.toLocaleString('zh-CN')} tokens</span>}
          </span>
        </button>
      </div>
      {hasChildren && (
        <ul
          id={`children-${node.id}`}
          className="span-children"
          hidden={!expanded}
        >
          {node.children.map((child) => (
            <SpanBranch
              key={child.id}
              node={child}
              selected={selected}
              onSelect={onSelect}
            />
          ))}
        </ul>
      )}
    </li>
  );
}

/** 长文本只在详情内换行，不挤占执行树的宽度。 */
function Preview({ title, value }: { title: string; value: string }) {
  return value ? (
    <section className="mt-7">
      <h3 className="mb-2 text-xs font-medium text-zinc-500">{title}</h3>
      <pre className="whitespace-pre-wrap break-words font-mono text-xs leading-6 text-zinc-600">
        {value}
      </pre>
    </section>
  ) : null;
}

function SpanDetails({ span }: { span: SpanNode }) {
  return (
    <article className="span-detail" aria-label="节点详情">
      <div className="mb-2 flex items-center justify-between text-xs">
        <span className="text-zinc-400">{kinds[span.kind] || span.kind}</span>
        <RunStatus value={spanStatus(span)} />
      </div>
      <h2 className="mb-6 break-words text-base font-medium">{span.name}</h2>
      <dl className="detail-fields">
        <dt>类型</dt>
        <dd className="font-mono text-xs">{span.kind}</dd>
        <dt>开始</dt>
        <dd>{dateTime(span.started_at)}</dd>
        <dt>结束</dt>
        <dd>{dateTime(span.ended_at)}</dd>
        <dt>耗时</dt>
        <dd>{duration(span.duration_ms)}</dd>
        {span.step !== null && span.step > 0 && (
          <>
            <dt>步数</dt>
            <dd>{span.step}</dd>
          </>
        )}
        <dt>输入 tokens</dt>
        <dd>{tokenCount(span.prompt_tokens)}</dd>
        <dt>输出 tokens</dt>
        <dd>{tokenCount(span.completion_tokens)}</dd>
        <dt>缓存 tokens</dt>
        <dd>{tokenCount(span.cached_tokens)}</dd>
        {span.estimated_tokens !== null && (
          <>
            <dt>预估 tokens</dt>
            <dd>{span.estimated_tokens.toLocaleString('zh-CN')}</dd>
          </>
        )}
        {span.error_code && (
          <>
            <dt>错误代码</dt>
            <dd className="text-rose-600">{span.error_code}</dd>
          </>
        )}
        {span.call_id && (
          <>
            <dt>工具调用</dt>
            <dd className="font-mono text-xs">{span.call_id}</dd>
          </>
        )}
        <dt>Span</dt>
        <dd className="font-mono text-xs">{span.id}</dd>
      </dl>
      {Object.keys(span.attributes).length > 0 && (
        <section className="mt-7 border-t border-zinc-100 pt-5">
          <h3 className="mb-3 text-xs font-medium text-zinc-500">属性</h3>
          <dl className="span-attributes">
            {Object.entries(span.attributes).map(([key, value]) => (
              <div key={key}>
                <dt>{key}</dt>
                <dd>
                  {typeof value === 'string'
                    ? value
                    : JSON.stringify(value, null, 2)}
                </dd>
              </div>
            ))}
          </dl>
        </section>
      )}
      <Preview title="输入预览" value={span.input_preview} />
      <Preview title="输出预览" value={span.output_preview} />
    </article>
  );
}

function RunMetadata({ run }: { run: RunDetail }) {
  return (
    <details className="config-disclosure border-b border-zinc-100">
      <summary>
        运行信息
        <ChevronRight size={14} className="text-zinc-400" />
      </summary>
      <dl className="detail-fields">
        {[
          ['运行', run.id],
          ['用户', run.owner_user_id],
          ['Workspace', run.workspace_id],
          ['会话', run.conversation_id],
          ['Agent', run.agent_id],
          ['Agent 版本', run.agent_version_id],
          ['用户消息', run.user_message_id],
          ['最终消息', run.final_message_id],
          ['供应商', run.provider],
          ['流协议', run.stream_protocol],
          ['工具集 hash', run.toolset_hash],
          ['创建', dateTime(run.created_at)],
          ['开始', dateTime(run.started_at)],
          ['结束', dateTime(run.ended_at)],
        ].map(([label, value]) => (
          <div key={label} className="contents">
            <dt>{label}</dt>
            <dd className="break-all text-xs">{value || '—'}</dd>
          </div>
        ))}
        <dt>工具集</dt>
        <dd className="font-mono text-xs">
          {run.toolset.length ? run.toolset.join(', ') : '—'}
        </dd>
        <dt>限制</dt>
        <dd className="whitespace-pre-wrap font-mono text-xs">
          {JSON.stringify(run.limits, null, 2)}
        </dd>
      </dl>
    </details>
  );
}

/** 运行与执行树一次读取；选中节点保存在 URL，刷新后可继续排查。 */
export default function RunDetailPage() {
  const { id = '' } = useParams<{ id: string }>();
  const [params, setParams] = useSearchParams();
  const load = useCallback((signal: AbortSignal) => getRun(id, signal), [id]);
  const { data, error, loading, reload } = useResource(load);
  const run = data?.run;
  const root = data?.span;
  const selected = root
    ? findNode(root, params.get('span') || '') || root
    : undefined;
  return (
    <Page
      title="运行追踪"
      actions={
        <Button
          variant="ghost"
          size="sm"
          aria-label="刷新运行详情"
          onClick={reload}
          disabled={loading}
        >
          <RefreshCw size={14} />
        </Button>
      }
    >
      <Link
        to="/runs"
        className="mb-7 inline-flex items-center gap-2 text-xs text-zinc-500 hover:text-zinc-950"
      >
        <ArrowLeft size={14} />
        全部运行
      </Link>
      {error ? (
        <div className="space-y-3">
          <ErrorNotice>{error}</ErrorNotice>
          <Button variant="ghost" onClick={reload}>
            重试
          </Button>
        </div>
      ) : loading ? (
        <section
          aria-busy="true"
          aria-label="加载运行详情"
          className="space-y-8"
        >
          <Skeleton className="h-16" />
          <Skeleton className="h-72" />
        </section>
      ) : run ? (
        <>
          <div className="run-summary">
            <div>
              <h2 className="text-base font-medium">{run.agent_name}</h2>
              <p className="mt-1 text-xs text-zinc-500">{run.user_email}</p>
            </div>
            <RunStatus value={run.status} />
            <span className="text-xs text-zinc-500">{run.model}</span>
          </div>
          <dl className="run-metrics">
            {[
              [duration(run.latency_ms), '总耗时'],
              [duration(run.active_ms), '活跃耗时'],
              [
                (run.prompt_tokens + run.completion_tokens).toLocaleString(
                  'zh-CN',
                ),
                'Tokens',
              ],
              [duration(run.ttft_ms), 'TTFT'],
              [String(run.steps), '步数'],
              [String(run.tool_calls), '工具调用'],
            ].map(([value, label]) => (
              <div key={label}>
                <dd>{value}</dd>
                <dt>{label}</dt>
              </div>
            ))}
          </dl>
          {(run.error || run.error_code || run.cancel_reason) && (
            <p className="mb-6 break-words text-sm text-rose-600">
              {run.error || run.error_code || run.cancel_reason}
            </p>
          )}
          <RunMetadata run={run} />
          {root && selected ? (
            <div className="trace-columns">
              <section className="span-tree" aria-label="执行树">
                <h2 className="mb-4 px-2 text-xs font-medium text-zinc-500">
                  执行树
                </h2>
                <ul>
                  <SpanBranch
                    key={id}
                    node={root}
                    selected={selected.id}
                    onSelect={(span) => setParams({ span })}
                  />
                </ul>
              </section>
              <SpanDetails span={selected} />
            </div>
          ) : (
            <p className="empty-state">
              {['queued', 'running', 'waiting_approval'].includes(run.status)
                ? '尚无执行记录'
                : '此轮发生在追踪上线之前，没有执行树'}
            </p>
          )}
        </>
      ) : null}
    </Page>
  );
}
