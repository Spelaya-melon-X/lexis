/// <reference types="node" />
import { svelte } from '@sveltejs/vite-plugin-svelte'
import { defineConfig } from 'vitest/config'

// Dev: `npm run dev` (порт 5173) проксирует API на Go-сервер (`go run ./cmd/server` или docker).
// Адрес бэкенда — LEXIS_API_URL (по умолчанию http://127.0.0.1:8090 — порт Lexis на хосте,
// см. LEXIS_PORT; 8080 на машине занят другим проектом).
// Prod: `npm run build` → dist/, Dockerfile встраивает его в бинарник (backend/web).
const api = process.env.LEXIS_API_URL ?? 'http://127.0.0.1:8090'

export default defineConfig({
  plugins: [svelte()],
  server: {
    proxy: {
      '/api': api,
      '/healthz': api,
    },
  },
  build: {
    outDir: 'dist',
    target: 'es2022',
  },
  test: {
    // Тестируем только чистую логику в .ts — DOM не нужен.
    include: ['src/**/*.test.ts'],
    environment: 'node',
  },
})
