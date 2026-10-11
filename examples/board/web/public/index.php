<?php

declare(strict_types=1);

// Errors go to the log. PHP startup warnings appear before this line runs, so the server must also
// start with display_errors off.
ini_set('display_errors', '0');

use Flowmark\Board\Assets;
use Flowmark\Board\Posts;
use Polyspec\Hyper\App;
use Polyspec\Orm\Config;

require __DIR__ . '/../vendor/autoload.php';
// The models are generated from the corpus catalog (orm-gen), which no autoloader maps to a file per class.
require __DIR__ . '/../src/Model/bootstrap.php';

// The corpus the board reads and writes: flowmark serve prints its port, and the token stands in the
// corpus' .flowmark/server.json. orm reaches the server through this DSN.
$dsn = getenv('FLOWMARK_DSN');
if (!is_string($dsn) || $dsn === '') {
    throw new RuntimeException('FLOWMARK_DSN must name the corpus: markdown://127.0.0.1:<port>/<corpus>?token=<token>');
}
// Server-side rendering serves the application at the root.
$basePath = getenv('BOARD_BASE_PATH');
$app = App::open(
    manifest: __DIR__ . '/../app/app.json',
    // `make board-server` builds the program; PHP renders with the native extension when it has loaded it.
    program: __DIR__ . '/../build/server',
    handlers: require __DIR__ . '/../app/handlers.php',
    timezone: '+09:00',
    basePath: is_string($basePath) ? $basePath : '',
    https: getenv('BOARD_HTTPS') === '1',
    frameAncestors: "'self'",
);
$app->bind(Posts::class, fn (): Posts => new Posts(Flowmark\Board\Model\connect($dsn, new Config()), fn (): int => time()));
$app->bind(Assets::class, fn (): Assets => Assets::fromManifest(__DIR__ . '/../build/manifest.json'));
$app->run();
