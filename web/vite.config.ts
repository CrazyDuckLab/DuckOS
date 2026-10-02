import { defineConfig } from "vitest/config";
import type { Plugin } from "vite";

/* Dev-only middleware that exposes every icon's SVG markup as JSON.
 * Used by the iconography-capture.js Playwright script to render the
 * spec sheet and size comparison sheet without going through the React
 * tree. Has no production impact — it only listens while `vite dev`
 * is running and the iconography capture is in flight. */
function iconPathsPlugin(): Plugin {
  return {
    name: "duckos-icon-paths",
    apply: "serve",
    configureServer(server) {
      server.middlewares.use("/__icon-paths.json", async (_req, res) => {
        try {
          const mod = await server.ssrLoadModule("/src/icons.tsx");
          const paths = mod.__iconPathsForCapture;
          if (!paths) {
            res.statusCode = 500;
            res.end("icon paths not exported");
            return;
          }
          res.setHeader("content-type", "application/json");
          res.end(JSON.stringify(paths));
        } catch (err) {
          res.statusCode = 500;
          res.end(String(err));
        }
      });
    },
  };
}

export default defineConfig({
  build: { target: "es2022", sourcemap: false, assetsInlineLimit: 0 },
  plugins: [iconPathsPlugin()],
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
    exclude: ["e2e/**", "node_modules/**"],
  },
});
