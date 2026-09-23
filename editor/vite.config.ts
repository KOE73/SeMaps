import { defineConfig } from "vite";
import dts from "vite-plugin-dts";
import { fileURLToPath, URL } from "node:url";

/**
 * Two build targets share one source tree.
 *
 *   default  — library mode: `dist/semaps-editor.js` + types, consumed by anyone
 *              who wants only the canvas.
 *   --mode app — the editor application, emitted into `host/app/`. The host
 *              serves it from its own folder at `/app/`; the workspace is served
 *              at the root, so the app reads models from `../`.
 *
 * In dev the workspace is mounted as the public dir, so `npm run dev` serves
 * real models without Go. Only saving and source lookup need the host, and
 * that is what the /api proxy is for. The workspace defaults to the bundled
 * example; point `SEMAPS_WORKSPACE` at another one.
 */
const workspaceDir =
  process.env.SEMAPS_WORKSPACE ?? fileURLToPath(new URL("../examples/workspace", import.meta.url));

export default defineConfig(({ mode }) => {
  const isApp = mode === "app";

  return {
    // In app mode the page is served from /app/, so assets must resolve
    // relatively — the host has no base-path rewriting.
    base: isApp ? "./" : "/",

    publicDir: workspaceDir,

    server: {
      port: 5177,
      proxy: {
        // Saving and source lookup are the host's job (host/server.go).
        "/api": {
          target: "http://localhost:8777",
          changeOrigin: true,
        },
      },
    },

    build: isApp
      ? {
          outDir: fileURLToPath(new URL("../host/app", import.meta.url)),
          // The models live in publicDir; the app never carries them.
          copyPublicDir: false,
          emptyOutDir: true,
          // The app build is committed so that run.cmd works without a Node
          // toolchain; a source map would add ~190 kB of churn per rebuild.
          sourcemap: false,
        }
      : {
          lib: {
            entry: fileURLToPath(new URL("src/index.ts", import.meta.url)),
            name: "SemapsEditor",
            formats: ["es"],
            fileName: () => "semaps-editor.js",
            cssFileName: "semaps-editor",
          },
          copyPublicDir: false,
          emptyOutDir: true,
          sourcemap: true,
        },

    plugins: isApp
      ? []
      : [
          dts({
            include: ["src"],
            rollupTypes: true,
          }),
        ],
  };
});
