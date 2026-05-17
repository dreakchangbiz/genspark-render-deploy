Custom local wrapper for `deanxv/genspark-playwright-proxy`.

What it changes:
- Replaces `/app/dist/index.js` with a locally patched version that avoids the Playwright crash we saw on `page.goto()`.
- Keeps the original upstream image as the base layer.

How to run:

```bash
cd /Volumes/DC/Codex/Genspark/genspark-playwright-proxy-custom
docker compose up -d --build
```

Useful commands:

```bash
docker compose logs -f
docker compose restart
docker compose down
```

Optional proxy support:

```bash
PROXY_URL="http://user:pass@host:port" docker compose up -d --build
```
