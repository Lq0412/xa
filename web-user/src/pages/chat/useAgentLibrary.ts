import { useEffect, useState } from 'react';
import {
  type Agent,
  addPublicAgent,
  errorMessage,
  getAgent,
  listAgentLibrary,
} from '@/services/api';

/** 列表由账号保存；直接打开公共 Agent 链接也会将它添加到对话列表。 */
export function useAgentLibrary(selectedID: string | null) {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [revision, setRevision] = useState(0);
  // biome-ignore lint/correctness/useExhaustiveDependencies: revision 用于保存后刷新和显式重试。
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError('');
    const load = async () => {
      const result = await listAgentLibrary(controller.signal);
      if (selectedID && !result.agents.some((item) => item.id === selectedID)) {
        const selected = await getAgent(selectedID, controller.signal);
        if (selected.visibility === 'public' && selected.status === 'active') {
          await addPublicAgent(selectedID, controller.signal);
          result.agents.push(selected);
        }
      }
      if (!controller.signal.aborted)
        setAgents(
          result.agents.sort(
            (a, b) => Number(b.is_default) - Number(a.is_default),
          ),
        );
    };
    void load()
      .catch((cause) => {
        if (!controller.signal.aborted) setError(errorMessage(cause));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [selectedID, revision]);
  return {
    agents,
    loading,
    error,
    refresh: () => setRevision((value) => value + 1),
  };
}
