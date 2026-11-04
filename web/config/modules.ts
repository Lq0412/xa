/** 管理端模块目录；导航与概览共用这份结构，状态表示实际功能进度。 */
export type AdminModule = {
  path: string;
  name: string;
  icon: string;
  description: string;
  available?: boolean;
  sections: {
    key: string;
    title: string;
    description: string;
    columns: string[];
  }[];
};

export const adminModules: AdminModule[] = [
  {
    path: '/agents',
    name: 'Agent 管理',
    icon: 'robot',
    available: true,
    description:
      '维护公共 Agent 的资料、提示词和可用状态，逐步接入模型与能力绑定。',
    sections: [],
  },
  {
    path: '/models',
    name: '模型与供应商',
    icon: 'api',
    description: '统一管理模型服务，为 Agent 和知识库提供可选择的模型能力。',
    sections: [
      {
        key: 'models',
        title: '模型配置',
        description:
          '配置文本、Realtime、Embedding、语音及重排序模型的参数、能力和可用状态。',
        columns: ['模型名称', '供应商', '能力', '状态', '更新时间'],
      },
      {
        key: 'providers',
        title: '供应商',
        description:
          '维护服务地址、协议和凭证引用，检测服务连接与实际模型能力。',
        columns: ['供应商', '协议', '服务地址', '连接状态', '更新时间'],
      },
    ],
  },
  {
    path: '/knowledge',
    name: '知识库',
    icon: 'database',
    description: '管理供 Agent 使用的知识资料，从文档上传到检索验证。',
    sections: [
      {
        key: 'bases',
        title: '知识库',
        description: '创建知识库，选择向量模型并管理可用范围。',
        columns: ['知识库名称', '向量模型', '文档数量', '状态', '更新时间'],
      },
      {
        key: 'documents',
        title: '文档与分块',
        description: '上传与解析文档，查看分块、处理状态和来源，维护索引。',
        columns: ['文档名称', '所属知识库', '处理状态', '分块数量', '更新时间'],
      },
      {
        key: 'retrieval',
        title: '检索验证',
        description: '输入查询，检查召回内容、重排序结果及引用来源。',
        columns: ['查询内容', '知识库', '检索状态', '耗时', '时间'],
      },
    ],
  },
  {
    path: '/tools',
    name: '工具与 MCP',
    icon: 'tool',
    description: '管理可供 Agent 调用的工具与 MCP 服务，以及各自的使用权限。',
    sections: [
      {
        key: 'catalog',
        title: '工具目录',
        description: '查看工具说明、参数、版本、权限与执行限制。',
        columns: ['工具名称', '来源', '版本', '状态', '更新时间'],
      },
      {
        key: 'mcp',
        title: 'MCP 服务',
        description: '管理 MCP 服务连接，发现工具并控制开放范围。',
        columns: ['服务名称', '连接方式', '连接状态', '工具数量', '更新时间'],
      },
    ],
  },
  {
    path: '/evaluation',
    name: '评价规则',
    icon: 'checkCircle',
    description: '维护学习评价的维度、规则与评分依据，供 Agent 按需使用。',
    sections: [
      {
        key: 'rules',
        title: '规则管理',
        description: '配置语法、发音和综合评价规则，保留规则版本。',
        columns: ['规则名称', '评价类型', '版本', '状态', '更新时间'],
      },
      {
        key: 'validation',
        title: '规则验证',
        description: '使用提交的文本或录音验证评价规则，查看评分依据。',
        columns: ['规则名称', '输入类型', '验证状态', '耗时', '时间'],
      },
    ],
  },
  {
    path: '/tasks',
    name: '任务管理',
    icon: 'schedule',
    description: '查看文档处理、记忆提取和评价任务的执行状态。',
    sections: [
      {
        key: 'runs',
        title: '任务记录',
        description: '查看任务进度、执行结果与失败原因，处理可重试任务。',
        columns: ['任务标识', '任务类型', '状态', '重试次数', '创建时间'],
      },
      {
        key: 'definitions',
        title: '任务类型',
        description: '查看已注册处理器、超时和重试策略。',
        columns: ['任务类型', '处理器', '超时', '重试策略', '状态'],
      },
    ],
  },
  {
    path: '/users',
    name: '用户与权限',
    icon: 'team',
    description: '管理账号状态和资源访问范围。登录与资源归属校验已在后端接入。',
    sections: [
      {
        key: 'users',
        title: '用户账号',
        description:
          '查询账号、查看角色并管理账号启停。用户管理接口与页面操作待开发。',
        columns: ['用户', '邮箱', '角色', '账号状态', '注册时间'],
      },
      {
        key: 'permissions',
        title: '资源权限',
        description: '查看模型、工具和知识库的开放范围，维护资源授权。',
        columns: ['资源名称', '资源类型', '开放范围', '状态', '更新时间'],
      },
    ],
  },
  {
    path: '/operations',
    name: '运行监控与审计',
    icon: 'monitor',
    description: '查看服务运行、调用错误和管理操作记录。',
    sections: [
      {
        key: 'runtime',
        title: '运行监控',
        description:
          '查看调用量、耗时、失败状态和资源用量；基础健康检查已在后端接入。',
        columns: ['运行标识', '能力', '状态', '耗时', '时间'],
      },
      {
        key: 'audit',
        title: '审计日志',
        description: '追踪配置变更、资源启停和权限调整等管理操作。',
        columns: ['操作人', '操作', '资源', '结果', '时间'],
      },
    ],
  },
];
