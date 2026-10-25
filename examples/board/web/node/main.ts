// Serves the board application with the Node server, as public/index.php serves it with PHP; the posts stand in a
// flowmark corpus that orm reaches through the DSN markdown://. Build this file with esbuild into build/node/server.mjs.
//
// Environment: FLOWMARK_DSN (markdown://127.0.0.1:<port>/<corpus>?token=<token>), BOARD_SESSIONS (an absolute session directory), BOARD_PORT (0
// lets the system assign the port; the server prints the port that it listens on), and as for PHP BOARD_BASE_PATH, BOARD_HTTPS=1 and BOARD_FRAME_ANCESTORS (HY-45).
import { mkdirSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { App, FileSessions } from '@polyspec/hyper-server';
import { connect } from './model/models.js';
import { handlers, type Services } from './handlers.js';
import { Posts } from './posts.js';

// The bundle is build/node/server.mjs inside the board directory.
const board = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const env = (name: string): string => {
  const value = process.env[name];
  if (value === undefined || value === '') throw new Error(`${name} is required`);
  return value;
};
const dsn = env('FLOWMARK_DSN');
const port = Number(env('BOARD_PORT'));
const sessionDirectory = env('BOARD_SESSIONS');
mkdirSync(sessionDirectory, { recursive: true, mode: 0o700 });

const app = await App.open<Services>({
  manifest: join(board, 'app', 'app.json'),
  // `make assets` builds the template files that the browser loads; the server renders with the same files.
  templates: { index: join(board, 'build', 'templates.index.json'), root: join(board, 'public') },
  handlers,
  timezone: '+09:00',
  basePath: process.env.BOARD_BASE_PATH ?? '',
  https: process.env.BOARD_HTTPS === '1',
  frameAncestors: process.env.BOARD_FRAME_ANCESTORS || "'self'",
});
const db = await connect(dsn);
app.bind('posts', () => new Posts(db, () => Math.floor(Date.now() / 1000)));
app.bind('assets', () => {
  const urls: unknown = JSON.parse(readFileSync(join(board, 'build', 'manifest.json'), 'utf8'));
  if (typeof urls !== 'object' || urls === null || !('hyper' in urls) || typeof urls.hyper !== 'string') throw new Error('the asset manifest has no hyper URL; run make assets');
  return { hyper: urls.hyper };
});
// The session cookie has the name that the PHP board uses, PHP's default session name.
const server = app.server(new FileSessions({ directory: sessionDirectory }), { files: join(board, 'public') });
server.listen(port, '127.0.0.1', () => {
  const address = server.address();
  if (address === null || typeof address === 'string') throw new Error(`the board server listens on ${String(address)}, not on a TCP port`);
  process.stdout.write(`board on http://127.0.0.1:${address.port}\n`);
});
