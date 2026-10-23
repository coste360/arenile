// Static "external website" for the cross-origin embed acceptance test
// (spec 28, M4): serves a lido's own page that embeds the widget with
// embed.js loaded from the Arenile instance, on a different origin.
//
//   E2E_HOST_PORT=8092 ARENILE_URL=http://localhost:8090 node fixtures/host-server.mjs
import { createServer } from 'node:http';

const port = Number(process.env.E2E_HOST_PORT ?? 8092);
const arenile = (process.env.ARENILE_URL ?? 'http://localhost:8090').replace(/\/$/, '');

const page = (intercept) => `<!doctype html>
<html lang="it">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Lido di prova - sito del lido</title>
    <style>
      body { margin: 0; font: 16px/1.5 Georgia, serif; color: #23313f; background: #fdf8ef; }
      header { background: linear-gradient(135deg, #0b4f6c, #0e6ba8); color: #fff; padding: 32px 16px; text-align: center; }
      header h1 { margin: 0; font-size: 32px; letter-spacing: .02em; }
      header p { margin: 4px 0 0; opacity: .85; }
      main { max-width: 1100px; margin: 0 auto; padding: 24px 16px 48px; }
      main > h2 { font-size: 22px; margin: 0 0 12px; }
    </style>
  </head>
  <body>
    <header><h1>Lido di prova</h1><p>Il sito del lido, su un dominio diverso da Arenile.</p></header>
    <main>
      <h2>Prenota online</h2>
      <div data-arenile="lido-demo" data-lang="it" data-title="Prenota il tuo posto"></div>
    </main>
    <script>
      window.__booked = null;
      window.__navigate = null;
      document.addEventListener('arenile:booked', function (e) { window.__booked = e.detail; });
      ${
        intercept
          ? `document.addEventListener('arenile:navigate', function (e) { e.preventDefault(); window.__navigate = e.detail.url; });`
          : ''
      }
    </script>
    <script src="${arenile}/embed.js" async></script>
  </body>
</html>`;

const handler = (req, res) => {
  const url = new URL(req.url ?? '/', 'http://host');
  if (url.pathname === '/healthz') {
    res.writeHead(200, { 'content-type': 'text/plain' }).end('ok');
    return;
  }
  if (url.pathname === '/' || url.pathname === '/lido.html') {
    // ?intercept=1: the site handles arenile:navigate itself (preventDefault).
    res.writeHead(200, { 'content-type': 'text/html; charset=utf-8' }).end(page(url.searchParams.get('intercept') === '1'));
    return;
  }
  res.writeHead(404).end('not found');
};

// Loopback only: 127.0.0.1 is the allowed origin of the test, localhost
// (which may resolve to ::1) is the NOT allowed one.
createServer(handler).listen(port, '127.0.0.1', () => {
  console.log(`host page on http://127.0.0.1:${port} (widget from ${arenile})`);
});
createServer(handler)
  .on('error', () => {}) // no IPv6 loopback: localhost resolves to 127.0.0.1
  .listen(port, '::1');
