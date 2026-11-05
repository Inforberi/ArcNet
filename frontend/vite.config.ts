import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
export default defineConfig({
  plugins: [react(),tailwindcss()],
  build: {target:'es2022',sourcemap:false},
  server: {proxy:{'/api':'http://127.0.0.1:8181','/sub':'http://127.0.0.1:8181'}}
});
