import { defineConfig } from 'vite'
import tailwindcss from '@tailwindcss/vite'
import webfontDownload from 'vite-plugin-webfont-dl'

export default defineConfig({
  plugins: [
    tailwindcss(),
    webfontDownload('https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;600;700&family=Geist:wght@300;400;500;600;700&display=swap'),
  ],
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
