# Render deployment

This deployment uses two Render services:

- `genspark2api`: public Web Service for OpenAI-compatible API traffic.
- `genspark-playwright-proxy`: Private Service used only by `genspark2api` for ReCaptcha token generation.

## Required secrets

Set these when Render prompts during Blueprint creation:

- `GS_COOKIE`: your Genspark cookie header. At minimum, `session_id=...`.
- `API_SECRET`: a long random bearer token. Clients must send it as `Authorization: Bearer <API_SECRET>`.

Optional:

- `MODEL_CHAT_MAP`: can be added later in the Render dashboard if model-to-chat binding is needed.
- `PROXY_URL`: set on `genspark-playwright-proxy` only if you want Playwright to use an upstream proxy.

## Deploy

1. Push this repository to GitHub, GitLab, or Bitbucket.
2. In Render, choose **Blueprints > New Blueprint Instance**.
3. Select the repository and `render.yaml`.
4. Fill in `GS_COOKIE` and `API_SECRET`.
5. Create the Blueprint.

After deploy, use:

```text
https://<genspark2api-service>.onrender.com/v1
```

with:

```http
Authorization: Bearer <API_SECRET>
```

The ReCaptcha proxy stays private and is wired through `RECAPTCHA_PROXY_HOSTPORT`.
