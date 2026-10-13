import { fileURLToPath } from "node:url";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

const gen = fileURLToPath(new URL("../gen/ts", import.meta.url));
const src = fileURLToPath(new URL("./src", import.meta.url));

export default defineConfig({
  plugins: [react(), tailwindcss()],
  clearScreen: false,
  resolve: {
    alias: { "@gen": gen, "@": src },
    dedupe: ["@bufbuild/protobuf"],
  },
  server: {
    port: 1420,
    strictPort: true,
    fs: { allow: [".", gen] },
  },
  build: { target: "es2022" },
  test: {
    projects: [
      { extends: true, test: { name: "logic", include: ["src/**/*.test.ts"], environment: "node" } },
      { extends: true, test: { name: "dom", include: ["src/**/*.test.tsx"], environment: "jsdom" } },
    ],
  },
});
