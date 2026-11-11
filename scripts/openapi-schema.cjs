const { readFileSync } = require('node:fs');

const serviceNames = {
  renzheng: 'auth',
  agentguanli: 'agents',
  agentku: 'agentLibrary',
  gongju: 'tools',
  jiyi: 'memory',
  huihua: 'conversations',
  yunxing: 'runs',
  shenpi: 'approvals',
  jiankangjiancha: 'health',
};

/** Umi 先把中文标签转为拼音，生成文件使用稳定的英文名。 */
function serviceName(tag = '') {
  return serviceNames[tag] ?? tag;
}

/** Umi 能读取 anyOf，但无法识别 OpenAPI 3.1 的数组形式 type。 */
function normalizeUnionTypes(value) {
  if (Array.isArray(value)) return value.map(normalizeUnionTypes);
  if (value === null || typeof value !== 'object') return value;
  const schema = Object.fromEntries(
    Object.entries(value).map(([key, item]) => [key, normalizeUnionTypes(item)]),
  );
  if (!Array.isArray(schema.type)) return schema;
  const { type, format, ...fields } = schema;
  return {
    ...fields,
    // 把 format 放在分支内，避免生成器把可空数字整个识别为 number。
    anyOf: type.map((member) => ({ ...schema, type: member, format })),
  };
}

/** Umi 通过 CommonJS 加载本地契约，源文件使用 YAML。 */
function registerSchemaLoader(parse) {
  require.extensions['.yaml'] = (module, filename) => {
    module.exports = normalizeUnionTypes(parse(readFileSync(filename, 'utf8')));
  };
}

module.exports = { registerSchemaLoader, serviceName };
