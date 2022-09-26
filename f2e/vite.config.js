import {defineConfig} from 'vite'
import path from 'path';
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
                entryFileNames: 'assets/[name].js',
                chunkFileNames: 'assets/[name].js',
                assetFileNames: `assets/[name].[ext]`,
            }
        }
    },
    server: {
        proxy: {
            '/readlog/wsapi': {
                target: 'ws://192.168.10.109:9198',
                changeOrigin: true,
                ws: true
            },
            '/readlog/list': {
                target: 'http://192.168.10.109:9198',
                changeOrigin: true,
            }
        }
    }
})
