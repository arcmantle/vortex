import { defineConfig } from 'vite';

export default defineConfig({
	build: {
		outDir: '../../../cmd/vortex/web/dist',
		emptyOutDir: true,
	},
	server: {
		port: 5173,
		proxy: {
			'/api': {
				target: 'http://localhost:7370',
				changeOrigin: true,
			},
			'/events': {
				target: 'http://localhost:7370',
				changeOrigin: true,
				// SSE requires the proxy to not buffer the response
				configure: (proxy) => {
					proxy.on('proxyRes', (proxyRes) => {
						proxyRes.headers['cache-control'] = 'no-cache';
					});
				},
			},
			'/handoff': {
				target: 'http://localhost:7370',
				changeOrigin: true,
			},
		},
	},
});
