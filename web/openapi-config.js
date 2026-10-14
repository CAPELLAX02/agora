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
  // Sanal transkript ve not düzeltme önizlemesi POST'tur ama bir şey değiştirmez: sorgu olarak
  // üretilir, önbelleği geçersiz kılmaz, sonuçlar girdilere göre önbellekte tutulur.
  endpointOverrides: [
    { pattern: 'simulateTranscript', type: 'query' },
    { pattern: 'previewGradeChange', type: 'query' },
  ],
}

export default config
