import { defineConfig } from 'vite';
import { spawn } from 'child_process';

function goBackend() {
	let proc;
	return {
		name: 'go-backend',
		configureServer() {
			proc = spawn('go', ['run', './cmd/vortex', 'run', '--dev', '--port', '7370', '--config', 'mock/dev.vortex'], {
				cwd: new URL('../../../', import.meta.url).pathname,
				stdio: 'inherit',
			});
			proc.on('error', (err) => console.error('[go-backend]', err.message));
		},
		buildEnd() {
			proc?.kill();
		},
	};
}

export default defineConfig({
	plugins: [goBackend()],
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
		},
	},
});
