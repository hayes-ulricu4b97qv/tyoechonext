# Typecho Next 架构

这个仓库的产品是使用 Go API 和 Vue 客户端的个人博客。服务打开自己的 SQLite 数据库，并提供下面列出的路由。运行、备份与恢复见 [operations.md](operations.md)。

## 目录

```text
cmd/server/          Go 服务入口、优雅退出
internal/config/     环境配置
internal/httpapi/    HTTP 路由、鉴权与 JSON API；auth.go、content.go、settings.go 按领域承载认证、内容和设置处理
internal/service/    业务规则；account.go、content.go、engagement.go、audit_sessions.go、validation.go 按领域拆分
internal/store/      SQLite 仓储；schema.go、accounts.go、content.go、engagement.go、audit_sessions.go、scan.go 分离建表、领域查询和扫描辅助
web/src/             Vue 3 + TypeScript 前端
web/dist/            Vite 构建产物（运行前生成）
```

## 路由

| 路径 | 用途 |
| --- | --- |
| `/setting` | 首次部署初始化、站点设置 |
| `/admin` | 管理端登录、统计、文章编辑 |
| `/user` | 用户端公开文章列表、搜索与 taxonomy 筛选 |
| `/user/{slug}` | 用户端文章详情与评论 |
| `/page/{slug}` | 用户端独立页面 |
| `/feed.xml`、`/sitemap.xml`、`/robots.txt` | Go 服务提供的公开订阅与索引入口 |
| `/api/v1/*` | 版本化 JSON API |
| `/healthz` | 存活检查，初始化前也返回正常 |
| `/readyz` | 就绪检查，初始化完成前返回 503 |

服务启动时打开 `APP_DATA_DIR/app.db`。首次启动会创建本应用的 schema，之后继续执行本应用已有的列升级。没有初始化记录时，页面请求转到 `/setting`，初始化成功后才开放 `/admin` 和 `/user`。

文章接口支持 `q`、`category`、`tag`、`month`、`order`、`page` 和 `pageSize` 参数。`order` 默认按 `published_at`，传入 `updated` 时按更新时间。显式传入 1 到 100 之外的 `pageSize` 会返回校验错误。公开接口只返回已发布且发布时间不晚于当前时刻的文章，内容编辑者可以带 `includeDraft=true` 读取草稿。登录 cookie 只携带随机令牌，数据库中保存 SHA-256 哈希、`users.id` 和过期时间，因此服务重启后，同一数据库上的登录会话仍然有效。会话寿命由 `APP_SESSION_TTL_MIN` 决定，缺省 1440 分钟。
文章和页面搜索使用词元索引，并按字面匹配用户输入，`%` 与 `_` 不会被当作通配符。分类和标签名称仍用带转义的 `LIKE`。

`/feed.xml` 只收录已发布文章，按发布时间排序，描述使用摘要或截断后的正文。`/sitemap.xml` 包含读者首页、已发布文章、公开分类与标签、已发布独立页面。`/robots.txt` 允许 `/`，禁止 `/admin` 和 `/setting`，站点 URL 存在时附上 `Sitemap`。Vue 用户端不生成这些文件。

审计事件写入 SQLite 的 `audit_events` 表，并通过 owner-only 的 `GET /api/v1/audit` 查询；文章、评论、媒体、用户、账户和站点的成功变更都会记录操作者、资源和必要的变更详情。

写请求采用双提交 CSRF 令牌：Go 服务设置非 HttpOnly 的 `app_csrf` cookie，Vue API 客户端复制到 `X-CSRF-Token`；初始化和登录是仅有的无需既有令牌的写入口，匿名评论仍不要求登录。

管理员会话还可以通过 `PUT /api/v1/settings/account` 更新账户邮箱，或在提交 `currentPassword` 与 `newPassword` 时修改密码。密码修改沿用 bcrypt 哈希，会话按 `users.id` 关联并在换密时撤销其他令牌。邮箱和密码都写在所有者的 `users` 记录上。

文章写入接口接受 `title`、`slug`、`excerpt`、`content`、`status`、`allowComment`、`categories`、`tags`，以及可选的 `publishAt` 和 `passphrase`。`categories` 与 `tags` 是 JSON 字符串数组；管理端表单里的逗号只是录入时的拆分。省略 `allowComment` 时，新文章默认允许评论，编辑文章则保留原设置。首次发布写入 `published_at`，之后再次发布保持该时间。`publishAt` 晚于当前时间时，匿名读者在到达该时间前看不到文章。`passphrase` 只写入哈希，响应不返回口令；未提供正确口令的匿名读者看不到正文。ID 和更新时间由后端生成或维护，存储模型不会直接成为 HTTP 请求模型。
编辑文章时如果省略 `slug`，服务会保留原有固定链接；新文章才会根据标题生成 slug，避免改标题时破坏已有链接。自动 slug 会保留 Unicode 字母和数字，因此中文标题、分类和标签可以生成稳定的可读路径。
管理端可从文章和独立页面列表打开预览；服务只会向当前有内容编辑权限的会话返回草稿，匿名用户仍会得到不存在响应。
文章和独立页面共享内容长度约束：标题最多 200 个字符、slug 最多 120 个字符、摘要最多 500 个字符、正文最多 500,000 个字符。

文章还可以提交 `categories` 和 `tags` 名称数组；Go 服务维护 `terms` 与 `post_terms` 关系表，`GET /api/v1/content/terms` 返回分类、标签及已发布文章数，内容编辑者可通过 `POST/PUT/DELETE /api/v1/content/terms` 管理词条。
文章列表支持 `category=<slug>` 和/或 `tag=<slug>` 筛选，同时提供两个参数时返回两个词条的交集；草稿仍只会在有内容编辑权限的会话中返回。
Vue 用户端会从公开词条接口生成筛选导航，分类、标签和搜索状态写入 URL，因此可以直接分享并支持浏览器前进后退。

独立页面使用 `pages` 表和独立 API：公开端通过 `GET /api/v1/content/pages/{slug}` 读取，内容编辑者通过 `GET/POST /api/v1/content/pages`、`PUT/DELETE /api/v1/content/pages/{id}` 管理。

评论通过文章路由提交：`POST /api/v1/content/posts/{slug}/comments` 按站点阅读设置决定结果。默认新评论进入待审核；所有者关闭“新评论需要审核”后，评论会直接公开。公开列表只返回 `approved` 评论。管理员可以通过 `/api/v1/content/comments?status=pending` 获取待审核列表，并用 `PUT /api/v1/content/comments/{id}` 将评论设为 `approved`、`pending` 或 `spam`，也可以删除评论。

所有者通过 `GET/PUT /api/v1/settings/reading` 管理阅读与评论：`postsPerPage`（1–50，公开文章列表未传 `pageSize` 时使用）、`commentsEnabled`、`commentsRequireModeration` 和 `commentsThreaded`。文章还可以单独设置 `allowComment`；站点或文章关闭评论时，创建评论会被拒绝。开启回复后，评论可以带 `parentId`，父评论必须是同一篇文章里已通过的评论，嵌套最多三层。公开状态接口会带上 `reading`，读者端据此分页、决定是否展示评论表单，并把回复按时间排成线程。

所有者还可以在站点设置里保存 `description` 和 `keywords`。匿名公开状态会带回这两个字段。描述非空时，`/feed.xml` 的频道描述使用这段文字；描述为空时仍回退为站点名加 ` latest posts`。编辑和审核员不能修改站点字段。

公开文章详情带有按发布时间排序的上一篇和下一篇，同一发布时间用文章 ID 做稳定次序，草稿不会成为邻居。`GET /api/v1/content/archive` 按站点 IANA 时区把已发布文章收成月份计数；`GET /api/v1/content/posts?month=YYYY-MM` 返回该月的公开列表。`order=published` 按发布时间返回文章。`GET /api/v1/content/comments/recent` 只返回已发布文章下的已通过评论。Vue 读者端用这些接口渲染站点描述、相邻文章、月份归档、最新文章和最近回复。

媒体由 Go 服务保存到 `APP_MEDIA_DIR`，元数据写入 SQLite。管理员通过 `POST /api/v1/media` 上传图片，服务只接受 JPEG、PNG、GIF 和 WebP，单文件上限 10 MB；公开文件通过 `/media/{id}` 读取，删除时同时清理元数据和文件。

用户保存在 `users` 表。初始化直接创建 `owner`；其他账户可以是 `editor` 或 `moderator`。编辑者负责文章和媒体，审核员负责评论，站点、账户和用户管理只对所有者开放。停用用户后，其持久会话会在下一次请求时失效。

所有者用户管理接口为 `GET/POST /api/v1/settings/users`、`PUT/DELETE /api/v1/settings/users/{username}`；密码只在服务端生成 bcrypt 哈希，HTTP 响应不会返回密码字段。更换所有者密码时会保留当前会话并撤销其他会话；工作人员被停用或删除时会清理其会话。
所有者通过用户更新接口提交新密码即可重置工作人员密码，服务会撤销该用户已有会话。

## Windows 本地运行

```powershell
cd web
npm ci
npm run build
cd ..
go run ./cmd/server
```

开发时可以并行运行 `go run ./cmd/server` 与 `cd web && npm run dev`，Vite 会把 `/api` 代理到 `localhost:8080`。
