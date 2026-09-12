import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import path from 'node:path';

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    rollupOptions: {
      output: {
        // 四大稳定块独立缓存：react/router 几乎不变；radix 一组；dnd 一组；lucide 一组。
        // 业务代码改动只 invalidate app chunk，vendor 复用浏览器缓存。
        manualChunks(id) {
          if (!id.includes('node_modules')) return;
          if (/[\\/]node_modules[\\/](react|react-dom|react-router-dom|scheduler)[\\/]/.test(id))
            return 'react-vendor';
          if (id.includes('@radix-ui')) return 'radix-vendor';
          if (id.includes('@dnd-kit')) return 'dnd-vendor';
          if (id.includes('lucide-react')) return 'lucide-vendor';
        },
      },
    },
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, 'src'),
      shared: path.resolve(__dirname, '../shared/src'),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': { target: 'http://localhost:9494', changeOrigin: true },
      '/healthz': { target: 'http://localhost:9494', changeOrigin: true },
    },
  },
});
