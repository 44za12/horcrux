import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

export default defineConfig({
  plugins: [svelte()],
  clearScreen: false,
  server: {
    port: 34115,
    strictPort: true,
  },
  envPrefix: ['VITE_', 'WAILS_'],
  build: {
    target: 'esnext',
    outDir: 'dist',
    minify: !process.env.WAILS_DEV,
  },
})
