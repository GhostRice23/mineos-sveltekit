// Custom server for handling WebSocket proxying
// This wraps the SvelteKit handler and proxies WebSocket connections to the API.
// The proxy logic itself lives in ./server/ws-proxy.js so it can be unit-tested.

import { createServer } from 'http';
import { WebSocketServer } from 'ws';
import { handler } from './build/handler.js';
import { createUpgradeHandler, toWebSocketBase } from './server/ws-proxy.js';

const PORT = parseInt(process.env.PORT || '3000', 10);
const HOST = process.env.HOST || '0.0.0.0';

// API base URL for proxying
const API_BASE = process.env.PRIVATE_API_BASE_URL || process.env.INTERNAL_API_URL || 'http://api:5078';
const API_KEY = process.env.PRIVATE_API_KEY || '';

// Convert HTTP URL to WebSocket URL
const WS_BASE = toWebSocketBase(API_BASE);

// Create HTTP server
const server = createServer(handler);

// Create WebSocket server (no server - we'll handle upgrades manually)
const wss = new WebSocketServer({ noServer: true });

// A WebSocketServer that never gets an 'error' listener throws on failure,
// which would take the whole web process down with it.
wss.on('error', (err) => {
	console.error('[WS Proxy] Server error:', err.message);
});

server.on('upgrade', createUpgradeHandler({ wss, wsBase: WS_BASE, apiKey: API_KEY }));

// Start server
server.listen(PORT, HOST, () => {
	console.log(`Listening on http://${HOST}:${PORT}`);
});
