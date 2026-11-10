import { useEffect, useState } from 'react';
import { ApiError } from '@/services/xa/request';

/** 页面读取在切换路由或筛选后取消；失败保留明确错误，不显示伪造空数据。 */
export function useResource<T>(load: (signal: AbortSignal) => Promise<T>) {
  const [revision, setRevision] = useState(0);
  const [result, setResult] = useState<{
    load: typeof load;
    revision: number;
    data?: T;
    error?: string;
  }>();
  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal).then(
      (data) => {
        if (!controller.signal.aborted) setResult({ load, revision, data });
      },
      (cause: unknown) => {
        if (controller.signal.aborted) return;
        if (!(cause instanceof ApiError)) throw cause;
        setResult({ load, revision, error: cause.message });
      },
    );
    return () => controller.abort();
  }, [load, revision]);
  const loading = result?.load !== load || result?.revision !== revision;
  return {
    data: loading ? undefined : result?.data,
    error: loading ? undefined : result?.error,
    loading,
    reload: () => setRevision((value) => value + 1),
  };
}
