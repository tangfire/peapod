import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks: {
          antd: ["antd"],
          pro: ["@ant-design/pro-components"],
          icons: ["lucide-react"]
        }
      }
    },
    chunkSizeWarningLimit: 1500
  },
  server: {
    proxy: {
      "/api": "http://127.0.0.1:8095",
      "/logout": "http://127.0.0.1:8095",
      "/static": "http://127.0.0.1:8095",
      "/favicon.png": "http://127.0.0.1:8095"
    }
  }
});
