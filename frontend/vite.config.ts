/// <reference types="node" />
import { svelte } from '@sveltejs/vite-plugin-svelte'
import { defineConfig } from 'vitest/config'

// Dev: `npm run dev` (порт 5173) проксирует API на Go-сервер (`go run ./cmd/server` или docker).
// Адрес бэкенда — LEXIS_API_URL (по умолчанию http://127.0.0.1:8090 — порт Lexis на хосте,
// см. LEXIS_PORT; 8080 на машине занят другим проектом).
// Prod: `npm run build` → dist/, Dockerfile встраивает его в бинарник (backend/web).
const api = process.env.LEXIS_API_URL ?? 'http://127.0.0.1:8090'

export default defineConfig(({ mode }) => ({
  plugins: [svelte()],
  // Vitest workers must use Svelte's client entry point when mounting in jsdom.
  ...(mode === 'test' ? { resolve: { conditions: ['browser'] } } : {}),
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
    // Компонентные тесты подключают jsdom через @vitest-environment.
    include: ['src/**/*.test.ts'],
    environment: 'node',
    server: { deps: { inline: ['svelte'] } },
  },
}))
