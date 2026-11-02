import { copyFileSync, mkdirSync } from 'node:fs';
mkdirSync('internal/web/static', {recursive:true});
copyFileSync('node_modules/htmx.org/dist/htmx.min.js', 'internal/web/static/htmx.min.js');
mkdirSync('third_party/htmx', {recursive:true});
copyFileSync('node_modules/htmx.org/LICENSE', 'third_party/htmx/LICENSE');

for (const file of ['qr-scanner.min.js','qr-scanner-worker.min.js']) {
 copyFileSync(`node_modules/qr-scanner/${file}`, `internal/web/static/${file}`);
}
mkdirSync('third_party/qr-scanner', {recursive:true});
copyFileSync('node_modules/qr-scanner/LICENSE', 'third_party/qr-scanner/LICENSE');
