import js from "@eslint/js";
import tseslint from "typescript-eslint";
import solid from "eslint-plugin-solid/configs/typescript";

// Флэт-конфиг: js + typescript-eslint (recommended) + solid-реактивность (typescript-preset).
// Минимально, но solid-aware — ловит реактивность-баги (destructure props и т.п.).
export default tseslint.config(
  { ignores: ["dist/**", "node_modules/**"] },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    files: ["src/**/*.{ts,tsx}"],
    ...solid,
  },
);
