# Typecho Next 架构

当前仓库已经增加一条独立的新运行链路，旧的 PHP Typecho 文件仅作为迁移参考保留。新服务不读取旧配置，也不提供旧路由兼容层。

## 目录

```text
cmd/server/          Go 服务入口、优雅退出
internal/config/     环境配置
internal/httpapi/    HTTP 路由、鉴权与 JSON API
internal/store/      SQLite 仓储、schema migration 与旧 app.json 导入
web/src/             Vue 3 + TypeScript 前端
web/dist/            Vite 构建产物（运行前生成）
```

## 路由

| 路径 | 用途 |
| --- | --- |
| `/setting` | 首次部署初始化、站点设置 |
| `/admin` | 管理端登录、统计、文章编辑 |
| `/user` | 用户端公开文章列表 |
| `/api/v1/*` | 版本化 JSON API |
| `/healthz` | 部署健康检查 |

服务启动时会检查 `APP_DATA_DIR/app.db`。首次启动会执行 schema migration；如果存在上一阶段的 `app.json`，会自动导入站点、管理员和文章。没有初始化记录时，页面请求统一转到 `/setting`，初始化成功后才开放 `/admin` 和 `/user`。

文章接口支持 `q`、`page` 和 `pageSize` 参数。公开接口只返回已发布文章，管理员会话可以读取草稿。登录 cookie 只携带随机令牌，数据库中保存 SHA-256 哈希和过期时间，因此服务重启后会话仍然有效。

## 本地运行

```bash
cd web
npm ci
npm run build
cd ..
go run ./cmd/server
```

开发时可以并行运行 `go run ./cmd/server` 与 `cd web && npm run dev`，Vite 会把 `/api` 代理到 `localhost:8080`。

## 后续迁移顺序

1. 把 SQLite 仓储替换为 PostgreSQL（多节点）时保持 API 契约不变。
2. 增加用户、权限、媒体和评论领域包，不把业务逻辑放回 HTTP handler。
3. 为文章详情、搜索和主题渲染增加独立的用户端路由。
4. 迁移完成后删除仓库中的旧 PHP 运行时与 `admin/`、`var/` 目录。
