import { execSync } from "child_process";
import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import tailwindcss from "@tailwindcss/vite";

const versionNumber = execSync("git describe --tags").toString().trim();
export default defineConfig({
  define: {
    "version": `"${versionNumber}"`,
  },
  plugins: [tailwindcss(), svelte()],
  server: {
    proxy: {
      "/api": "http://localhost:8080",
      "/auth": "http://localhost:8080",
      "/ws": {
        target: "ws://localhost:8080",
        ws: true,
      },
    },
  },
});
