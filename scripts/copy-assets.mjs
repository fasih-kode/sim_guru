import { copyFileSync, mkdirSync } from 'node:fs';
mkdirSync('internal/web/static', {recursive:true});
copyFileSync('node_modules/htmx.org/dist/htmx.min.js', 'internal/web/static/htmx.min.js');
mkdirSync('third_party/htmx', {recursive:true});
copyFileSync('node_modules/htmx.org/LICENSE', 'third_party/htmx/LICENSE');
