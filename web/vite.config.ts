import { execSync } from "child_process";
import { readFileSync, writeFileSync } from "fs";
import { resolve } from "path";
import { defineConfig, type Plugin } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import tailwindcss from "@tailwindcss/vite";

const gitVersion = execSync("git describe --tags").toString().trim();

function swVersionPlugin(): Plugin {
  return {
    name: "sw-version",
    writeBundle({ dir }) {
      if (!dir) return;
      const swPath = resolve(dir, "sw.js");
      try {
        const content = readFileSync(swPath, "utf-8");
        writeFileSync(swPath, content.replace("__SW_VERSION__", gitVersion));
      } catch {}
    },
  };
}

export default defineConfig({
  define: {
    "version": `"${gitVersion}"`,
  },
  plugins: [tailwindcss(), svelte(), swVersionPlugin()],
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
