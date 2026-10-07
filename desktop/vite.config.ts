import { fileURLToPath } from "node:url";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

const gen = fileURLToPath(new URL("../gen/ts", import.meta.url));

export default defineConfig({
  plugins: [react()],
  clearScreen: false,
  resolve: {
    alias: { "@gen": gen },
    dedupe: ["@bufbuild/protobuf"],
  },
  server: {
    port: 1420,
    strictPort: true,
    fs: { allow: [".", gen] },
  },
  build: { target: "es2022" },
  test: { environment: "node" },
});
