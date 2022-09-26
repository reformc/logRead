import {defineConfig} from 'vite'
import vue from '@vitejs/plugin-vue'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite';
import {ArcoResolver} from 'unplugin-vue-components/resolvers';

// https://vitejs.dev/config/
export default defineConfig({
    plugins: [
        vue(),
        AutoImport({
            resolvers: [ArcoResolver()],
        }),
        Components({
            resolvers: [
                ArcoResolver({
                    importStyle:'less',
                    sideEffect: true
                })
            ]
        })
    ],
    build: {
        outDir: '../',
        emptyOutDir: false,
        minify: true,
        rollupOptions:{
            output: {
                entryFileNames: 'readlog/[name].js',
                chunkFileNames: 'readlog/[name].js',
                assetFileNames: `readlog/[name].[ext]`,
            }
        }
    },
    server: {
        proxy: {
            '/readlog/wsapi': {
                target: 'ws://wy.365ymd.com:19179',
                changeOrigin: true,
                ws: true
            },
            '/readlog/list': {
                target: 'http://wy.365ymd.com:19179',
                changeOrigin: true,
            }
        }
    }
})
