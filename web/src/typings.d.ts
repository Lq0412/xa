declare module '*.css';
declare module '*.less';
declare module '*.scss';
declare module '*.sass';
declare module '*.svg';
declare module '*.png';
declare module '*.jpg';
declare module '*.jpeg';
declare module '*.gif';
declare module '*.bmp';
declare module '*.tiff';
declare module '*.md' {
  const content: string;
  export default content;
}

declare namespace XA {
  /** 服务端认证后的用户身份，客户端不得自行指定角色。 */
  type User = API.UserResponse;

  /** Agent 的稳定身份和当前可用状态。 */
  type Agent = API.AgentDefinition;

  /** 服务端完整配置；尚未提供编辑界面的字段仍需原样提交。 */
  type Configuration = NonNullable<API.AgentRequest['configuration']>;

  /** 工具绑定保留服务端扩展字段，只编辑启用状态和审批策略。 */
  type ToolBinding = API.ToolBinding;

  /** 不可变配置版本，保存时携带其 id 进行并发校验。 */
  type AgentVersion = API.AgentConfigResponse;
}
