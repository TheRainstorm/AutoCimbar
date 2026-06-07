import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    strictPort: true,
    port: 5173,
  },
  build: {
    outDir: process.env.VITE_AUTOCIMBAR_LITE === '1' ? '../lite/frontend/dist-lite' : 'dist',
    emptyOutDir: true,
  },
})
