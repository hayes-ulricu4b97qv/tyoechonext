# Typecho Next

Typecho Next is a Go API with a Vue 3 + TypeScript client.

## Product entry points

- `/setting` — first deployment setup and site settings
- `/admin` — administrator login, dashboard and writing
- `/user` — public reader experience
- `/user/{slug}` — public article detail and comments
- `/page/{slug}` — public static page
- `/feed.xml` / `/sitemap.xml` — public feed and search indexing endpoints
- `/api/v1` — versioned JSON API
- `/healthz` / `/readyz` — liveness and readiness checks

The service redirects every page to `/setting` until setup creates the site and the owner account. Runtime data is stored in SQLite at `APP_DATA_DIR/app.db`. Opening a data directory creates that database and applies this application's schema, including column upgrades for databases it already created.

## Quick start

```bash
cd web
npm ci
npm run build
cd ..
go run ./cmd/server
```

Open <http://localhost:8080/setting> and complete the setup form. For development, run Vite with `cd web && npm run dev`; `/api` is proxied to the Go service.

## Container

```bash
docker build -t typecho-next .
docker run --rm -p 8080:8080 -v typecho-data:/app/data typecho-next
```

在受限网络的构建环境中，把环境提供的代理 CA 作为 BuildKit secret 传入：

```bash
docker build --secret id=proxy_ca,src="$CODEX_PROXY_CERT" -t typecho-next .
```

See [docs/architecture.md](docs/architecture.md) for the module boundaries.

## Runtime requirements

* Go 1.22 or newer
* Node.js 20 or newer for the Vue build
* GCC or another C toolchain for local SQLite builds (the Docker image includes it)
* A writable `APP_DATA_DIR` (the container uses `/app/data`)

SQLite is the single-node persistence adapter. The repository interface keeps the API boundary ready for another adapter when multi-node deployment is needed.

Article listing accepts `q`, `page`, and `pageSize`. Sessions are persisted in SQLite with only a SHA-256 token hash stored server side. Each session references `users.id`, so a login remains valid for a later request against the same database.
Search terms are treated literally, including `%` and `_`.

Posts support comma-separated categories and tags. The Go service stores them in normalized taxonomy tables and exposes published term counts through `GET /api/v1/content/terms`. Content editors can manage terms with `POST/PUT/DELETE /api/v1/content/terms`.
Post lists can be filtered with `category=<slug>` and/or `tag=<slug>`; when both are present, the API returns their intersection.
The Vue reader exposes published categories, tags, and search terms as shareable URL filters.
Article details render Markdown content in Vue and sanitize generated HTML before insertion.
The Go service validates content limits: titles up to 200 characters, slugs up to 120, excerpts up to 500, and bodies up to 500,000 characters.
Content editors can create independent pages through `GET/POST /api/v1/content/pages` and update or delete them through `/api/v1/content/pages/{id}`.
When editing an article without a slug, the existing permalink is preserved; only new articles derive a slug from the title. Automatic slugs preserve Unicode letters and digits, so non-Latin titles and taxonomy names keep stable readable URLs.
Editors can preview drafts from the admin list; draft routes remain unavailable to anonymous readers.

Go directly serves `/feed.xml`, `/sitemap.xml`, and `/robots.txt` from published posts, pages, taxonomy pages, and site settings.

Owner accounts can inspect management history through `GET /api/v1/audit`; successful content, media, comment, user, account, and site changes record the actor and resource.
Write requests use a double-submit CSRF token; the Vue API client copies the token from the non-HttpOnly cookie into `X-CSRF-Token`. Initial setup and login are the only unauthenticated write entry points that do not require a prior token.

文章详情支持评论：默认先进入待审核状态，管理员在 `/admin` 审核后才会公开显示。所有者可以在 `/setting` 调整每页文章数、关闭评论、改为直接公开，或允许最多三层回复。单篇文章也可以关闭评论。

管理员也可以在 `/admin` 上传 JPEG、PNG、GIF 或 WebP 图片，文件保存在 `APP_MEDIA_DIR`，单文件上限为 10 MB。

所有者可以在 `/setting` 创建编辑者和审核员账户；权限由 Go API 校验，停用账户会立即阻止后续请求。更换所有者密码时会保留当前会话并撤销其他会话；工作人员被停用或删除时会清理其会话。
所有者也可以在用户列表中为工作人员重置密码，旧会话会立即失效。

管理员登录后可在 `/setting` 更新账户邮箱；提交当前密码和新密码即可轮换管理员密码。账户变更写入 `users` 表中的所有者记录。

## Contributing

Please see [CONTRIBUTING](CONTRIBUTING.md) for details.
