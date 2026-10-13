# 运行、备份与恢复

Typecho Next 是使用 Go、Vue 和 SQLite 的个人博客，仅支持 Windows 开发和部署。支持直接运行 Go 服务或编译后的 `.exe` 程序，数据和媒体保存在本地目录。

## 构建与运行

需要 Windows、PowerShell、Go 1.27.2 及以上版本、Node.js 22，以及已加入 PATH 的 MinGW-w64 GCC。SQLite 构建需要开启 CGO（`$env:CGO_ENABLED = '1'`）。在项目根目录执行：

```powershell
$env:CGO_ENABLED = '1'
.\tasks.ps1 build
.\bin\typecho-next.exe
```

构建脚本输出 `bin/typecho-next.exe` 和 `web/dist`。在项目根目录启动程序，访问 `http://localhost:8080/setting` 完成初始化。发布时同时携带程序和前端目录，或通过 `APP_WEB_DIR` 指定前端的绝对路径。

PowerShell 任务：`dev` 启动 Vite，`api-run` 启动 Go 服务，`web-build` 构建前端，`build` 构建前后端，`clean` 清理 `bin` 和 `web/dist`。例如 `.\tasks.ps1 api-run`。开发时在另一个终端执行 `.\tasks.ps1 dev`。

程序从环境变量读取配置，**不会自动加载 `.env` 文件**。`.env.example` 是配置参考；通过 PowerShell 或进程管理工具设置变量，例如 `$env:APP_DATA_DIR = 'C:\Typecho\data'`。可使用 Windows 任务计划程序配置启动与重启，并设置正确工作目录。程序不是直接注册到 Windows 服务控制管理器的服务，需要使用兼容的服务包装器才能按 Windows 服务运行。

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `APP_PORT` | `8080` | HTTP 监听端口 |
| `APP_DATA_DIR` | `./data` | SQLite 数据库目录 |
| `APP_MEDIA_DIR` | `APP_DATA_DIR/uploads` | 上传媒体目录 |
| `APP_WEB_DIR` | `./web/dist` | 前端构建目录 |
| `APP_VERSION` | `0.1.0` | 状态接口和日志显示的版本 |
| `APP_COOKIE_SECURE` | `false` | HTTPS 部署设置为 `true` |
| `APP_TRUSTED_PROXIES` | 未设置 | 可信反向代理的逗号分隔 CIDR；仅来自这些地址的 `X-Forwarded-For` 会参与来源识别 |
| `APP_SESSION_TTL_MIN` | `1440` | 会话有效期（分钟）；非正值、格式错误或超出 `time.Duration` 范围时回退到 1440 |
| `APP_WRITE_TIMEOUT` | `5m` | HTTP 响应总时限；应覆盖备份快照生成、压缩和下载所需时间 |
| `APP_LOGIN_FAILURE_LIMIT` | `5` | 同一客户端地址 10 分钟内的登录失败上限；成功登录会清零，超限后会在校验密码前拒绝请求 |
| `APP_COMMENT_RATE_LIMIT` | `30` | 同一来源每分钟的评论数上限 |
| `APP_PASSWORD_RESET_LIMIT` | `5` | 同一来源 15 分钟内的密码找回请求上限 |
| `APP_PASSWORD_RESET_ACCOUNT_LIMIT` | `1` | 同一有效账户 15 分钟内可发送的找回邮件上限；抑制跨来源邮件轰炸，超限时仍返回与未知账户相同的响应 |
| `APP_SMTP_HOST` | 未设置 | SMTP 主机；未设置时密码找回不可用 |
| `APP_SMTP_PORT` | `587`（STARTTLS）或 `465`（隐式 TLS） | SMTP 端口 |
| `APP_SMTP_USERNAME` / `APP_SMTP_PASSWORD` | 未设置 | 可选 SMTP 登录凭据，必须同时设置 |
| `APP_SMTP_FROM` | 未设置 | 发件地址；配置 SMTP 时必填 |
| `APP_SMTP_TLS_MODE` | `starttls` | `starttls` 或 `implicit`，连接必须使用 TLS |

数据库使用 `PRAGMA user_version` 标记 schema 版本。程序只接受当前版本，不迁移旧数据库或旧版本备份；schema 发生破坏性变化后，停止程序并删除 `APP_DATA_DIR/app.db`、`app.db-wal` 和 `app.db-shm`，再启动完成全新初始化。数据库和媒体目录分开存储；删除数据库会移除文章、账户和媒体索引，旧媒体文件不会自动登记到新站点。

服务管理器的工作目录可能与项目目录不同，生产环境建议为三个目录变量设置绝对路径。运行账户需要读写数据库与媒体目录、读取前端目录。通过反向代理提供 HTTPS 时，设置 `APP_COOKIE_SECURE=true`，代理应保留请求的 Cookie 和 CSRF 请求头。直接通过 HTTP 开发时使用 `false`。

登录、评论和密码找回限流默认按 TCP 对端地址计数。若反向代理需要区分访客，应只把实际代理地址加入 `APP_TRUSTED_PROXIES`；服务仅信任这些代理转发的 `X-Forwarded-For`，不会采信其他来源自行提交的该请求头。代理应覆盖或正确追加此头，避免客户端伪造地址。

`/healthz` 表示进程存活；`/readyz` 检查初始化状态及存储可用性，未完成初始化时返回 503。服务收到退出信号后会等待正在处理的请求，最多 10 秒。

密码找回要求配置完整 SMTP 投递：STARTTLS 默认使用 587 端口，隐式 TLS 使用 465 端口。邮件发送到用户账户登记的邮箱，包含 30 分钟有效的一次性口令；发件凭据只从进程环境读取，不写入数据库或日志。未配置 SMTP 时，所有找回请求都返回相同的服务不可用响应。找回请求同时按来源和有效账户限流；每账户限流只在邮件成功投递后保留，投递失败可立即重试。

待审核评论会进入所有者的站内通知历史；SMTP 已配置时也会尝试发送到站点邮箱。站内通知接口仅所有者可读，支持分页，保留最近 90 天且最多 1000 条。邮件发送失败不会撤回已经保存的评论，失败原因只写服务日志，站内通知仍保留。

## 备份

所有者登录后，可以在管理页顶部下载备份，也可以使用带登录 Cookie 的 `GET /api/v1/backup`。ZIP 包含 `app.db` 和数据库快照引用的 `media/` 文件。尚未上传媒体的站点可以生成只有数据库的备份。临时上传文件和未登记的文件不包含在内。

数据库使用 SQLite `VACUUM INTO` 生成一致快照，媒体依据快照中的记录归档。ZIP 完整生成后才开始下载；媒体缺失或无法读取时返回 `backup_failed`，不会将残缺备份作为成功下载。备份时避免删除媒体；数据库与媒体文件不是跨资源事务快照。临时目录需要容纳数据库快照和完整压缩包。`APP_WRITE_TIMEOUT` 覆盖整个 HTTP 请求，包含快照生成、压缩和发送；备份较大或下载链路较慢时，应同时提高应用超时与反向代理超时，留出生成和完整传输的时间。

备份包含账户哈希、会话、评论邮箱等私密数据，应限制访问并保存在服务器之外。

## 恢复

1. 停止博客服务，保留当前数据和媒体目录的完整副本，以便回退。
2. 将备份解压到独立临时目录。确认有 `app.db`，媒体位于 `media/`。
3. 将解压得到的 `app.db` 放入新的空 `APP_DATA_DIR`，将 `media/` 的内容放入新的空 `APP_MEDIA_DIR`。没有媒体时创建空目录。不要将旧数据库的 `app.db-wal` 或 `app.db-shm` 混入恢复目录。
4. 配置服务使用这些新目录，确保运行账户拥有访问权限，再启动服务。
5. 检查 `/readyz`、所有者登录、文章正文、评论和已上传图片。必要时在站点设置中调整站点 URL。

恢复会回到备份时刻的账户、密码和会话状态。不要将数据库恢复到备份时刻却继续使用一份不匹配的媒体目录。

## 发布检查

可执行的 Windows 浏览器验收使用 `cd web` 后运行 `npm run test:e2e`，覆盖浏览器操作、权限、备份恢复和重启；说明见 [e2e-testing.md](e2e-testing.md)。

发布前在 Windows 执行 `go test ./...`、`go vet ./...` 和 `.\tasks.ps1 build`，并使用临时数据目录验收初始化、登录、文章发布、评论审核、图片上传和备份恢复。首次初始化后验证重启仍能读取数据与持久会话。

RC 应对应固定提交和明确版本号，例如 `0.1.0-rc.1`。发布时将 `APP_VERSION` 设置为该版本并记录提交。`.\tasks.ps1 clean` 只清理程序和前端构建目录，保留博客数据；需要重新构建后才能运行完整页面。
