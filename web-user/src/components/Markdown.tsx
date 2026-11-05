import {
  type ComponentProps,
  isValidElement,
  type ReactNode,
  useRef,
  useState,
} from 'react';
import ReactMarkdown from 'react-markdown';
import rehypeHighlight from 'rehype-highlight';
import remarkGfm from 'remark-gfm';
import { Icon } from '@/components/Icon';
import { errorMessage } from '@/services/api';
import 'highlight.js/styles/github.css';

const languageNames: Record<string, string> = {
  js: 'JavaScript',
  javascript: 'JavaScript',
  jsx: 'JSX',
  ts: 'TypeScript',
  typescript: 'TypeScript',
  tsx: 'TSX',
  py: 'Python',
  python: 'Python',
  go: 'Go',
  bash: 'Bash',
  sh: 'Shell',
  shell: 'Shell',
  json: 'JSON',
  md: 'Markdown',
  markdown: 'Markdown',
  html: 'HTML',
  css: 'CSS',
  sql: 'SQL',
};

function languageOf(children: ReactNode) {
  const child = [children].flat().find((node) => isValidElement(node));
  if (!child) return '代码';
  const className = String(
    (child.props as { className?: string }).className || '',
  );
  const match = /language-([\w#+.-]+)/.exec(className);
  if (!match) return '代码';
  const token = match[1].toLowerCase();
  return languageNames[token] || match[1];
}

/** 从已渲染代码节点读取纯文本，高亮后的 span 不影响复制内容。 */
function CodeBlock({ children }: { children?: ReactNode }) {
  const ref = useRef<HTMLPreElement>(null);
  const [copied, setCopied] = useState(false);
  const [error, setError] = useState('');
  const copy = async () => {
    setError('');
    try {
      await navigator.clipboard.writeText(ref.current?.textContent || '');
      setCopied(true);
    } catch (cause) {
      setError(errorMessage(cause));
    }
  };
  return (
    <div className="my-3 overflow-hidden rounded-xl border border-zinc-200 bg-white">
      <div className="flex items-center justify-between border-b border-zinc-200 px-3 py-1.5">
        <span className="text-xs text-zinc-500">{languageOf(children)}</span>
        <button
          type="button"
          className="inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-xs text-zinc-500 hover:bg-zinc-100"
          onClick={() => void copy()}
          aria-label="复制代码"
        >
          <Icon name={copied ? 'check' : 'copy'} size={14} />
          {copied ? '已复制' : '复制'}
        </button>
      </div>
      <pre ref={ref}>{children}</pre>
      {error && (
        <span role="alert" className="block px-3 pb-2 text-xs text-rose-700">
          {error}
        </span>
      )}
    </div>
  );
}

const components: ComponentProps<typeof ReactMarkdown>['components'] = {
  pre: CodeBlock,
  a: ({ href, children }) => (
    <a href={href} target="_blank" rel="noopener noreferrer">
      {children}
    </a>
  ),
  table: ({ children }) => (
    <div className="my-3 max-w-full overflow-x-auto">
      <table>{children}</table>
    </div>
  ),
};

/** 聊天和 SOUL 共用安全的 GFM 渲染，禁止执行原始 HTML。 */
export function Markdown({ children }: { children: string }) {
  return (
    <div className="markdown">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[rehypeHighlight]}
        components={components}
        skipHtml
      >
        {children}
      </ReactMarkdown>
    </div>
  );
}
