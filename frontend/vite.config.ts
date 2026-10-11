import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    // Bind loopback by default; override with `vite --host` if needed.
    host: "127.0.0.1",
  },
  // @solana/web3.js expects Node globals: `global` and the `buffer` module.
  // Alias `buffer` to the npm polyfill (Vite otherwise externalizes the Node
  // built-in in the browser).
  resolve: {
    alias: {
      buffer: "buffer/",
    },
  },
  define: {
    global: "globalThis",
  },
  build: {
    outDir: "dist",
    sourcemap: false,
  },
});
