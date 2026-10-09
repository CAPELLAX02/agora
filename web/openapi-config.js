// RTK Query istemcisi sözleşmeden üretilir: make web-codegen. Üretilen dosya elle
// düzenlenmez, CI sözleşmeyle uyumlu olduğunu denetler.

/** @type {import('@rtk-query/codegen-openapi').ConfigFile} */
const config = {
  schemaFile: '../contracts/openapi.yaml',
  apiFile: './src/shared/api/baseApi.ts',
  apiImport: 'baseApi',
  outputFile: './src/shared/api/generated.ts',
  exportName: 'agoraApi',
  hooks: { queries: true, lazyQueries: true, mutations: true },
  tag: true,
}

export default config
