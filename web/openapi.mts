import { createRequire } from 'node:module';
import { resolve } from 'node:path';
import { generateService } from '@umijs/openapi';
import { parse } from 'yaml';

const require = createRequire(import.meta.url);
const {
  registerSchemaLoader,
  serviceName,
} = require('../scripts/openapi-schema.cjs');
registerSchemaLoader(parse);

await generateService({
  schemaPath: resolve(import.meta.dirname, '../docs/api/openapi.yaml'),
  serversPath: resolve(import.meta.dirname, 'src/services'),
  projectName: 'generated',
  requestLibPath: "import { request } from '@/services/xa/request'",
  requestOptionsType: '{ signal?: AbortSignal; anonymous?: boolean }',
  hook: { customClassName: serviceName },
});
