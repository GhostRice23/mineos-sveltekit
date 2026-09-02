import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig, loadEnv, type Plugin } from 'vite';
import { WebSocketServer, WebSocket } from 'ws';
// Plain JS helper shared with server.js, which runs as-is in the image and cannot
// import TypeScript.
import { closeSafely } from './wsCloseCode.js';
// The production proxy's same-origin guard, reused so the dev server is not the
// soft spot. server/ws-proxy.js is plain JS for the same reason wsCloseCode.js is.
import { isTrustedUpgradeOrigin } from './server/ws-proxy.js';

const allowedHostsEnv = process.env.VITE_ALLOWED_HOSTS ?? '';
const allowedHosts = allowedHostsEnv
	.split(',')
	.map((host: string) => host.trim())
	.filter((host: string) => host.length > 0);
const allowAnyHost =
	process.env.VITE_ALLOW_ANY_HOST === 'true' || process.env.VITE_ALLOW_ANY_HOST === '1';

// Load all env vars (including non-VITE_ prefixed) from .env for use in plugins
const allEnv = { ...loadEnv('development', process.cwd(), ''), ...process.env };

/**
 * Vite plugin to proxy WebSocket connections in dev mode.
 * Mirrors the logic in server.js for production.
 */
function wsProxyPlugin(): Plugin {
	const API_BASE =
		allEnv.PRIVATE_API_BASE_URL || allEnv.INTERNAL_API_URL || 'http://api:5078';
	const API_KEY = allEnv.PRIVATE_API_KEY || '';
	const WS_BASE = API_BASE.replace(/^http/, 'ws');
	const WS_PROXY_PATHS = ['/api/v1/admin/shell/ws'];

	function shouldProxy(path: string) {
		return WS_PROXY_PATHS.some((p) => path === p || path.startsWith(p + '?'));
	}

	function parseAuthToken(cookieHeader: string | undefined) {
		if (!cookieHeader) return null;
		const match = cookieHeader.match(/auth_token=([^;]+)/);
		return match?.[1] || null;
	}

	return {
		name: 'ws-proxy',
		configureServer(server) {
			const wss = new WebSocketServer({ noServer: true });

			server.httpServer?.on('upgrade', (req, socket, head) => {
				const url = new URL(req.url || '/', `http://${req.headers.host}`);
				const path = url.pathname;

				if (!shouldProxy(path)) return; // Let Vite handle its own HMR WebSocket

				// Same cross-site WebSocket hijacking guard as the production proxy,
				// and for the same reason: the handshake below carries the operator's
				// cookie and forwards it upstream as a Bearer token, so any page a
				// signed-in developer visits could otherwise open the admin shell as
				// them. Checked before the cookie is read.
				if (
					!isTrustedUpgradeOrigin({
						origin: req.headers.origin,
						host: req.headers.host,
						forwardedHost: req.headers['x-forwarded-host'] as string | undefined,
						configuredOrigin: allEnv.ORIGIN ?? null
					})
				) {
					console.error(
						`[WS Proxy Dev] Blocked cross-origin upgrade for ${path}: ` +
							`Origin="${req.headers.origin ?? ''}" Host="${req.headers.host ?? ''}"`
					);
					socket.write('HTTP/1.1 403 Forbidden\r\n\r\n');
					socket.destroy();
					return;
				}

				const token = parseAuthToken(req.headers.cookie);
				const targetUrl = `${WS_BASE}${path}${url.search}`;

				console.log(`[WS Proxy Dev] Connecting to: ${targetUrl}`);

				const upstreamHeaders: Record<string, string> = {};
				if (API_KEY) upstreamHeaders['X-Api-Key'] = API_KEY;
				if (token) upstreamHeaders['Authorization'] = `Bearer ${token}`;
				if (req.headers['sec-websocket-protocol']) {
					upstreamHeaders['Sec-WebSocket-Protocol'] = req.headers['sec-websocket-protocol'];
				}

				const upstream = new WebSocket(targetUrl, { headers: upstreamHeaders });

				upstream.on('open', () => {
					console.log(`[WS Proxy Dev] Connected to upstream`);
					wss.handleUpgrade(req, socket, head, (clientWs) => {
						clientWs.on('message', (data, isBinary) => {
							if (upstream.readyState === WebSocket.OPEN) {
								upstream.send(data, { binary: isBinary });
							}
						});
						upstream.on('message', (data, isBinary) => {
							if (clientWs.readyState === WebSocket.OPEN) {
								clientWs.send(data, { binary: isBinary });
							}
						});
						// Same reserved-close-code trap as the production proxy in
						// server.js; see wsCloseCode.js.
						clientWs.on('close', (code, reason) => closeSafely(upstream, code, reason));
						upstream.on('close', (code, reason) => closeSafely(clientWs, code, reason));
						clientWs.on('error', () => closeSafely(upstream));
						upstream.on('error', () => closeSafely(clientWs));
					});
				});

				upstream.on('error', (err) => {
					console.error(`[WS Proxy Dev] Failed to connect:`, err.message);
					socket.destroy();
				});
			});
		}
	};
}

export default defineConfig({
	plugins: [sveltekit(), wsProxyPlugin()],
	server: {
		host: true,
		allowedHosts: allowAnyHost ? true : allowedHosts.length > 0 ? allowedHosts : undefined
	}
});
