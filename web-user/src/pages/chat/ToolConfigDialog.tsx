import type { FormEvent, ReactNode } from 'react';
import { Dialog } from '@/components/Dialog';
import { Button } from '@/components/ui/button';
import type { AgentTool, RuntimeLimits, ToolBinding } from '@/services/tools';

const fieldClass =
  'h-9 w-full rounded-lg border border-zinc-200 px-2.5 text-sm';
const limitFields = [
  ['max_model_turns', '最大模型轮次'],
  ['max_tool_calls', '最大工具调用次数'],
  ['max_parallel_tools', '最大并行工具数'],
] as const;

/** 弹层共享提交与错误状态，字段按工具能力或运行限制显示。 */
function SettingsForm({
  title,
  description,
  error,
  busy,
  children,
  onClose,
  onSubmit,
}: {
  title: string;
  description?: string;
  error: string;
  busy: boolean;
  children: ReactNode;
  onClose: () => void;
  onSubmit: (fields: FormData) => void;
}) {
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!busy) onSubmit(new FormData(event.currentTarget));
  };
  return (
    <Dialog title={title} onClose={onClose} busy={busy}>
      <form className="space-y-4 px-5 pb-5" onSubmit={submit}>
        {description && (
          <p className="text-sm leading-5 text-zinc-600">{description}</p>
        )}
        <fieldset className="grid gap-3" disabled={busy}>
          {children}
        </fieldset>
        {error && (
          <p role="alert" className="text-sm text-rose-600">
            {error}
          </p>
        )}
        <div className="flex justify-end gap-2">
          <Button variant="ghost" disabled={busy} onClick={onClose}>
            取消
          </Button>
          <Button type="submit" disabled={busy}>
            {busy ? '保存中' : '保存'}
          </Button>
        </div>
      </form>
    </Dialog>
  );
}

/** 工具弹层只写入有改动的字段，继承的限制和未展示的配置继续由后端解析。 */
export function ToolConfigDialog({
  tool,
  description,
  hasTimezone,
  error,
  busy,
  onClose,
  onSave,
}: {
  tool: AgentTool;
  description?: string;
  hasTimezone: boolean;
  error: string;
  busy: boolean;
  onClose: () => void;
  onSave: (changes: Partial<ToolBinding>) => void;
}) {
  const timezone = tool.configuration.timezone ?? 'Asia/Shanghai';
  const timezones = hasTimezone
    ? [
        ...new Set(['UTC', ...Intl.supportedValuesOf('timeZone'), timezone]),
      ].sort()
    : [];
  return (
    <SettingsForm
      title={`配置${tool.title}`}
      description={description}
      error={error}
      busy={busy}
      onClose={onClose}
      onSubmit={(fields) => {
        const changes: Partial<ToolBinding> = {};
        const maxCalls = Number(fields.get('max_calls'));
        const timeout = Number(fields.get('timeout_ms'));
        const approval = fields.get('approval');
        const selectedTimezone = fields.get('timezone');
        if (
          (approval === 'always' || approval === 'never') &&
          approval !== tool.approval
        ) {
          changes.approval = approval;
        }
        if (maxCalls !== tool.max_calls) changes.max_calls = maxCalls;
        if (timeout !== tool.timeout_ms) changes.timeout_ms = timeout;
        if (
          typeof selectedTimezone === 'string' &&
          selectedTimezone !== timezone
        ) {
          changes.configuration = { timezone: selectedTimezone };
        }
        onSave(changes);
      }}
    >
      <label className="grid gap-1.5 text-sm">
        审批
        <select
          className={fieldClass}
          name="approval"
          defaultValue={tool.approval}
          disabled={!tool.approval_overridable}
        >
          <option value="always">需确认</option>
          <option value="never">自动运行</option>
        </select>
      </label>
      <label className="grid gap-1.5 text-sm">
        每次运行最多调用次数
        <input
          className={fieldClass}
          name="max_calls"
          type="number"
          min={1}
          step={1}
          defaultValue={tool.max_calls}
          required
        />
      </label>
      <label className="grid gap-1.5 text-sm">
        单次超时（毫秒）
        <input
          className={fieldClass}
          name="timeout_ms"
          type="number"
          min={1}
          max={60000}
          step={1}
          defaultValue={tool.timeout_ms}
          required
        />
      </label>
      {hasTimezone && (
        <label className="grid gap-1.5 text-sm">
          时区
          <select
            className={fieldClass}
            name="timezone"
            defaultValue={timezone}
          >
            {timezones.map((name) => (
              <option key={name} value={name}>
                {name}
              </option>
            ))}
          </select>
        </label>
      )}
    </SettingsForm>
  );
}

/** 保留未显示的运行限制，避免修改某一项时重置其他限制。 */
export function RuntimeLimitsDialog({
  limits,
  error,
  busy,
  onClose,
  onSave,
}: {
  limits: RuntimeLimits;
  error: string;
  busy: boolean;
  onClose: () => void;
  onSave: (limits: RuntimeLimits) => void;
}) {
  return (
    <SettingsForm
      title="运行限制"
      error={error}
      busy={busy}
      onClose={onClose}
      onSubmit={(fields) => {
        const updated = { ...limits };
        for (const [name] of limitFields)
          updated[name] = Number(fields.get(name));
        if (limitFields.every(([name]) => limits[name] === updated[name]))
          onClose();
        else onSave(updated);
      }}
    >
      {limitFields.map(([name, label]) => (
        <label key={name} className="grid gap-1.5 text-sm">
          {label}
          <input
            className={fieldClass}
            name={name}
            type="number"
            min={1}
            step={1}
            defaultValue={limits[name]}
            required
          />
        </label>
      ))}
    </SettingsForm>
  );
}
