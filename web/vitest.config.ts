import { defineConfig } from "vitest/config";

// Отдельный vitest-конфиг (не наследуем vite.config с solid-плагином): юнит-тесты покрывают
// ЧИСТЫЕ хелперы (api/format) — без DOM/JSX, environment=node, без лишних зависимостей (jsdom).
export default defineConfig({
  test: {
    environment: "node",
    include: ["src/**/*.test.ts"],
  },
});
