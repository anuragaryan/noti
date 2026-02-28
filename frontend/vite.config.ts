import { defineConfig } from 'vite'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [tailwindcss()],
  resolve: {
    alias: {
      '@': '/src',
      '@wails': '/wailsjs',
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
