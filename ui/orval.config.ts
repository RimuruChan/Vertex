import { defineConfig } from 'orval'

export default defineConfig({
  vertex: {
    input: '../server/docs/swagger.json',
    output: {
      target: './src/generated/api/vertex.ts',
      schemas: './src/generated/api/model',
      client: 'axios-functions',
      mode: 'single',
      clean: true,
      prettier: true,
      override: {
        mutator: {
          path: './src/api/http.ts',
          name: 'request',
        },
      },
    },
  },
})
