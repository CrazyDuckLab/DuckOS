import { defineConfig } from "vitest/config";

export default defineConfig({
  build: { target: "es2022", sourcemap: false, assetsInlineLimit: 0 },
  plugins: [],
  server: {
    host: "127.0.0.1",
    fs: {
      allow: [".", "../pkg/brand/mark.txt", "../pkg/brand/mark.txt?raw"],
    },
    proxy: {
      "/api": {
        target: process.env.DUCKOS_API_TARGET ?? "http://127.0.0.1:8080",
        changeOrigin: true,
        // Local development only: match the API's strict same-origin boundary.
        // Production assets are served by the API and do not use this proxy.
        headers: {
          origin:
            process.env.DUCKOS_API_ORIGIN ?? "http://127.0.0.1:8080",
        },
      },
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    restoreMocks: true,
    clearMocks: true,
  },
});