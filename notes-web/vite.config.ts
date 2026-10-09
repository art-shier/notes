import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
export default defineConfig({ plugins: [react()], server: { port: 5173, strictPort: true, proxy: {'/agent':{target:process.env.NOTES_API_PROXY||'http://127.0.0.1:8000'}, '/api':{target:process.env.NOTES_API_PROXY||'http://127.0.0.1:8000'}} }, build: { sourcemap: true, rollupOptions: { output: { manualChunks(id) { if(id.includes('node_modules') && (id.includes('tiptap')||id.includes('prosemirror'))) return 'editor-engine'; } } } } });
