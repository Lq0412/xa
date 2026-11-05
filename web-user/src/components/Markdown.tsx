import { type ComponentProps, type ReactNode, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import rehypeHighlight from 'rehype-highlight';
import remarkGfm from 'remark-gfm';
import { errorMessage } from '@/services/api';
import { Icon } from './Icon';
import 'highlight.js/styles/github.css';

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
    <div className="markdown-code-block">
      <button
        type="button"
        className="code-copy-button"
        onClick={() => void copy()}
        aria-label="复制代码"
      >
        <Icon name={copied ? 'check' : 'copy'} size={14} />
        {copied ? '已复制' : '复制'}
      </button>
      <pre ref={ref}>{children}</pre>
      {error && (
        <span role="alert" className="message-status">
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
