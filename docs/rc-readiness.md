# RC 准备记录

本项目按个人博客定位验收，采用 Go、Vue 和 SQLite，仅支持 Windows，通过原生 `.exe` 程序部署。

## 本轮改动

- 移除原有容器构建文件和部署说明，新增原生运行、配置和恢复说明。
- 备份在完整 ZIP 生成后才返回成功，归档引用来自数据库快照，媒体缺失会明确失败。
- PowerShell 任务脚本替代 Makefile，`clean` 清理程序和前端构建目录，保留博客数据。
- 修复首次设置向严格 JSON 接口发送多余字段的问题。
- Vite 开发代理补齐媒体和就绪检查路径。
- CI 仅在 Windows 执行 Go 测试、静态检查、前端构建和 `.exe` 构建。

## 已验证（2026-10-11）

- Windows：`go test ./...`、`go vet ./...`、`npm run build`、原生服务构建通过。
- Edge 无头浏览器：初始化、发布文章、图片上传及显示、备份下载、匿名阅读、匿名评论和审核通过，未捕获前端运行异常。
- 原生服务重启后：文章、已审核评论、图片和所有者账户保持可用；浏览器登录、保存站点设置和 `/readyz` 检查通过。
- 备份恢复回归测试：解压到空目录，打开恢复数据库并启动 HTTP 服务，验证文章内容、已有登录会话和媒体原始字节。
- 备份失败回归测试：数据库引用的媒体缺失时返回 500 `backup_failed`；无媒体站点可以备份，未登记的临时上传文件不会混入归档。
- 新增可重复执行的 Windows Playwright 浏览器验收及 CI 测试报告上传，覆盖详情见 [e2e-testing.md](e2e-testing.md)。修复错误阅读口令缺少提示的问题，并验证正确口令重试。
- 完整执行 `npm run test:e2e`：21/21 个流程通过，未捕获浏览器运行异常；包括设置分区、保存状态和离开提醒、恢复到空目录、浏览器删除资源、移动端布局和实际分页内容校验。

浏览器和运行验收使用 Windows 上的临时数据目录。实际远端 CI 结果应以工作流运行记录为准。

## 后续改进与验证（2026-10-12）

- 密码找回改为发送到用户账户邮箱，SMTP 支持强制 STARTTLS 或隐式 TLS；SMTP 未配置时明确返回不可用，找回响应不区分账户是否存在，发送失败会清除对应的一次性口令。
- 增加密码找回来源限流；进程内限流器清理过期来源并限制键数量。站内通知接口增加分页，通知历史清理 90 天前记录并限制最多 1000 条。
- 登录失败限流按客户端地址统计，并在调用密码校验前拦截已超限来源，避免换用户名绕过限制或持续触发 bcrypt。
- 支持显式配置可信反向代理 CIDR 后解析 `X-Forwarded-For`，未受信任的对端无法伪造限流来源；HTTP 响应增加防嵌入、引用来源和权限策略头。
- 客户端地址解析会合并重复的 `X-Forwarded-For` 头并从右向左沿代理链解析；头部超过 4096 字节或 32 跳时回退到可信代理地址，避免重复头绕过限流识别和无界解析。
- 文章列表与站点地图查询只读取摘要列，正文只在详情和 RSS 流程中按需加载。
- 读者端的独立页面导航和分类/标签筛选现在会按接口的 `total` 读完分页结果，不再在第 20 页静默截断；Playwright 用 101 个页面和分类确认第二页项目确实可见。
- 并发限流回归覆盖 100 个同时到达的请求只放行配置额度；`go test -race ./internal/service ./internal/httpapi` 通过，并加入 Windows CI。
- 密码重置口令不再进入 SQLite 通知正文或所有者通知界面。评论邮件发送失败时评论仍正常提交，通知继续保存在站内。
- 修复邮件卡片与评论审核卡片共用测试选择器的问题，并将通知区拆成独立 Vue 组件。
- 公开 HTML 壳补上 description、keywords、Open Graph、canonical；管理页和草稿预览输出 `noindex, nofollow`。文章和页面正文仍在脚本运行前输出，并对 HTML 特殊字符转义。
- 文章支持最多 50 项自定义字段，校验字段名、单值与总长度；文章修订包含字段快照，更新时省略字段会保留、空对象会清除。列表和口令未解锁的详情不返回字段，读者端只按文本展示。新 schema 直接创建字段列，不迁移旧数据库。
- 公开媒体读取和备份归档共用 `Lstat` 与已打开文件句柄的 `SameFile` 校验，拒绝符号链接目标及打开期间被替换的文件；回归测试覆盖公开媒体与备份路径。
- 密码及文章阅读口令明确校验 bcrypt 的 72 字节输入上限；超长输入返回 422，不进入服务端哈希错误。密码重置在消费一次性令牌前完成此校验，API 回归测试确认超长尝试后仍可用原令牌重置。
- 密码找回在来源限流外新增账户级 15 分钟限制，超限返回相同的接受响应，投递失败会清除冷却状态；回归测试覆盖跨来源限流与投递失败后重试。
- 未配置 SMTP 时，找回接口会在账户查询和账户级限流前统一返回相同的不可用响应，避免同一已知账户跨来源重复请求被账户限流提前改成接受响应，从而暴露账户是否存在；`TestPasswordResetRequiresConfiguredMailerWithoutAccountDisclosure` 覆盖首次、跨来源重复及不存在的账户。
- `/api/v1` 下未知路径与错误 HTTP 方法现在也使用稳定的 JSON 错误信封；405 响应保留 `Allow`。`TestAPIFallbackErrorsUseJSONEnvelope` 覆盖 404、405、错误码和中文消息。
- SQLite 启动改为严格 schema 版本策略：新库在事务中创建完整 schema 并设定 `user_version=2`；无版本号的非空旧库和不支持的版本在未被改动前即被拒绝。相关 store 测试、`go test ./...`、`go vet ./...`、race 检查和 Playwright E2E（24/24）通过。
- `tasks.ps1 clean` 会先检查 `web/dist` 与 `bin` 的根目录和内部重解析点，再统一清理，避免跟随目录联接或校验失败后只清掉部分目标。隔离临时副本验证了普通目录、嵌套 junction 和根 junction。
- Windows CI 明确限制为 `contents: read`，checkout 不持久保存仓库凭据，作业设 30 分钟上限，降低依赖安装和浏览器测试获得写权限或无限挂起的风险。
- Go 安全公告扫描发现 Go 1.27.1 标准库中 8 项可达漏洞，修复版为 1.27.2；项目最低 Go 版本、Windows CI 和运维要求现统一提升至 1.27.2。`golang.org/x/crypto` 从 v0.28.0 升级到 v0.58.0。Go 1.27.2 下 `govulncheck` 报告 0 项可达漏洞；唯一剩余公告是该模块中未导入的旧 `openpgp` 包，属于不可达的模块级结果。CI 已加入此扫描，并对 Web 依赖执行高危级别 `npm audit`。
- RSS 回归测试跨过 100 条分页边界并确认包含全部 103 篇已发布文章；独立页面和 taxonomy 的第二页仍由现有 E2E 数据集验证。
- 评论分页 API 测试覆盖 205 条评论的三页读取，验证总数、排序边界、无重复及跨页回复父 ID；读者端按 `total` 提供继续加载。
- 共用分页入口拒绝 0、负数、非整数页码及溢出 offset，同时接受最大安全 offset；覆盖所有 API 列表分页路由。
- Windows 本地验证：Go 1.27.2 下 `go test ./...`、`go vet ./...`、`go test -race ./internal/service ./internal/httpapi`、`govulncheck`、`npm run build`、`npm audit --audit-level=high`、`git diff --check` 通过；CI 工作流通过 `actionlint` 校验；Playwright 流程 24/24 通过。E2E 会构建前端和原生 Windows 服务，并覆盖字段编辑、XSS 文本显示、字段修订恢复、跨页导航与分类、文章/页面 Markdown 媒体插入预览、备份恢复、重启、权限与浏览器运行异常。

以上是当前工作区的验证结果，尚未代表远端 CI 已通过。正式宣布 RC 前仍需将最终代码固定为提交，并确认该提交的 CI 结果。

2026-10-12 增量复验中，直连 `https://vuln.go.dev/index/modules.json.gz` 遇到 TLS/EOF；改用兼容 Go 漏洞库 API 的 `https://storage.googleapis.com/go-vulndb` 后，`govulncheck v1.1.4` 完成扫描，报告 0 项可达漏洞和 1 项不可达的模块级公告。同期 `go test ./...`、`go vet ./...`、`go test -race ./internal/service ./internal/httpapi` 和 `git diff --check` 通过。CI 仍使用官方默认地址，远端工作流结果待确认。

## 搜索边界与查询成本（2026-10-12）

- 参考 `ck/opencode-2/V2_HTTP_API_AUDIT.md` 的参数与错误契约审查方式，为文章、页面和词条搜索统一设置 200 个 Unicode 字符上限；超限返回 422 `validation_error`，读者搜索框提示并限制相同长度。
- 搜索词元候选改由单条分组查询读取，再对候选正文做字面校验，避免每个唯一词元各执行一次 SQL。保留 `%`、`_` 的字面匹配行为，无 schema 变更。
- 本轮 Windows 验证：`go test ./...`、`go vet ./...`、`go test -race ./internal/service ./internal/store ./internal/httpapi`、`npm run build`、Playwright E2E 25/25 和 `git diff --check` 通过；超限边界由 `internal/httpapi/search_test.go` 覆盖，键盘焦点状态由真实浏览器流程验证。

## 站点地址与键盘访问（2026-10-12）

- 站点 URL 现在校验为根域名地址：拒绝路径、查询、片段和 URL 用户信息，避免拼出的 feed、sitemap、密码重置与 canonical 链接指向错误位置。初始化校验失败不会写入站点；更新失败保留现有 URL。
- 恢复输入框的键盘焦点轮廓，并单独覆盖标题编辑框；E2E 使用浏览器 Tab 导航确认读者搜索框和提交按钮均可见聚焦。
- 本轮 Windows 验证：`go test ./...`、`go vet ./...`、`go test -race ./internal/service ./internal/store ./internal/httpapi`、`npm run build`、Playwright E2E 25/25 和 `git diff --check` 通过。

站点地址回归覆盖路径、查询、片段和凭据四类无效 URL；也验证首次设置不会因校验失败写入初始化状态，站点更新失败后原地址保持不变。前端在设置表单中明确提示只填写协议和域名。

## 发布时间精度与归档邻文（2026-10-12）

- 数据库时间统一写成固定宽度的 UTC 纳秒格式，SQLite 文本排序与真实时间顺序一致。公开文章列表、文章详情、归档、分类/标签计数和近期评论均按完整时间精度排除尚未到发布时间的文章。
- 文章详情的上一篇/下一篇改为两条带复合索引的 SQL 邻居查询，按发布时间和文章 ID 稳定排序，不再为每篇详情加载全部公开文章元数据。新库增加发布时间索引并写入 `user_version=2`；旧版本按既有策略拒绝启动，需删除数据库重建。
- 回归测试覆盖小数秒排序、同一发布时间的 ID 次序、定时内容的公开可见性，以及前后邻居查询使用发布时间索引。
- Windows 本地验证：`go test ./...`、`go vet ./...`、`go test -race ./internal/service ./internal/store ./internal/httpapi` 和 `git diff --check` 通过。

## JSON 请求媒体类型校验（2026-10-12）

- JSON API 现在解析完整的 `Content-Type` 媒体类型，只接受 `application/json`（允许标准参数与大小写变体），拒绝前缀相似类型、格式错误的参数和重复头部，避免请求体解释不一致。
- 超过 JSON 请求体上限时，所有 JSON 写接口返回 HTTP 413 和 `request_too_large`；在上限内的格式错误仍返回 400 `invalid_request`。集成测试通过真实初始化路由验证超限响应。
- 回归测试覆盖接受与拒绝的媒体类型边界、重复头部与超限请求。验证结果：`go test ./...`、`go vet ./...`、目标包 race 检查、Playwright E2E 25/25 和 `git diff --check` 通过。

## API 查询参数契约（2026-10-12）

- API 现在拒绝已定义标量参数的重复值，避免服务端静默选择第一个值；`includeDraft` 和 `includePending` 只接受 `true` 或 `false`，格式错误的百分号编码返回 400，参数值无效返回 422。
- 回归覆盖全部已定义的标量参数、布尔值边界和畸形查询编码。验证通过：`go test ./...`、`go vet ./...`、目标包 race 检查、Playwright E2E 25/25 和 `git diff --check`。

## 固定候选版本

代码仍在工作区，尚未创建 RC 标签。正式宣布 RC 前，将本轮最终代码固定为提交，确认对应提交的 CI 通过，并使用明确版本号（例如 `0.1.0-rc.1`）构建发布包。运行时通过 `APP_VERSION` 显示该版本。

备份下载前应避免删除媒体。数据库和本地媒体不是同一个事务，文件缺失时会拒绝备份；详细步骤见 [operations.md](operations.md)。

## 按月归档 SQL 范围筛选（2026-10-12）
- 月份筛选改为 SQLite 发布时间半开区间查询，继续遵循站点时区的自然月边界，并利用 `posts_status_published_idx`，避免先读取全部已发布文章元数据再在 Go 中筛选。
- 回归覆盖起点包含、终点排除、纳秒精度、草稿与未来排期排除，以及洛杉矶 2024 年 3 月 DST 切换前后的 HTTP 归档归属。
- 本轮 Windows 本地复验：`go test ./...`、`go vet ./...`、`go test -race ./internal/service ./internal/store ./internal/httpapi`、Playwright E2E 25/25 和 `git diff --check` 通过；远端 CI 结果仍以工作流记录为准。

## 公开正文服务端 Markdown 渲染（2026-10-12）
- 文章和独立页面的首个 HTML 响应现包含 Goldmark GFM 渲染的语义正文标记；不运行浏览器脚本也能读到标题、段落、链接和图片，客户端随后照常挂载交互界面。
- 使用 Goldmark 默认安全渲染器，不启用原始 HTML；危险 URL（包括字符实体混淆的 `javascript:`）不会进入输出。口令文章与未来文章正文仍由公开可见性规则挡在 HTML 外。
- `TestPublicShellRendersSafeMarkdownWithoutJavaScript` 覆盖文章和页面渲染、图片链接、恶意 HTML、危险链接及 16 路并发请求。Windows 本地复验：`go test ./...`、`go vet ./...`、`go test -race ./internal/service ./internal/store ./internal/httpapi`、Playwright E2E 25/25 与 `git diff --check` 通过。

## 前端兜底路由方法约束（2026-10-12）
- SPA 前端兜底改为 Go ServeMux 的 `GET /` 模式；GET 页面请求与 HEAD 可用，POST、PUT、PATCH、DELETE、OPTIONS 等其他方法返回 405，不再被误当成页面请求。
- `TestFrontendFallbackOnlyAcceptsGetAndHead` 覆盖根路径和读者路径在初始化前后的不支持方法 405，以及 HEAD 无响应体。
- Windows 本地复验：`go test ./...`、`go vet ./...`、`go test -race ./internal/service ./internal/store ./internal/httpapi`、Playwright E2E 25/25 与 `git diff --check` 通过。

## 无脚本读者首页文章列表（2026-10-12）
- `/user` 首屏服务端输出文章标题、摘要、发布时间、分类/标签、总数和分页链接；搜索表单使用 GET，筛选及分页链接保留 URL 状态，JavaScript 客户端从同一组参数恢复当前页。
- 每页数量沿用站点 `postsPerPage`，无效值回退到 10，单页最多 50 篇；公开 SQL 不读取正文，并排除草稿与未来排期文章。标题、摘要、分类名和链接值均转义或编码。
- `TestPublicShellRendersSafePaginatedPostListWithoutJavaScript` 覆盖首页、搜索/分类/月筛选、分页、未来与草稿排除及恶意 HTML 转义；`TestPublicShellRendersAnEmptyPostState` 覆盖空列表状态。
- Windows 本地复验：`go test ./...`、`go vet ./...`、`go test -race ./internal/service ./internal/store ./internal/httpapi`、`npm run build`、Playwright E2E 25/25 和 `git diff --check` 通过。

## 独立页面的未来发布时间边界（2026-10-12）
- 公开页面列表、详情 API、服务端详情 HTML 和站点地图现在统一隐藏草稿、缺失发布时间和未来发布时间页面；无效或不可见的页面详情返回 404。管理端带 `includeDraft=true` 的列表和预览不受影响。
- `TestVisiblePageToPublicUsesFullTimestampPrecision` 验证发布时间边界；`TestPublicDetailShellReturnsNotFoundForUnavailableContent` 覆盖页面 API、HTML、站点地图及草稿/未来内容。
- 完整验证现已通过：`go test ./...`、`go vet ./...`、目标包 race 检查及 Playwright E2E 25/25。

## 页面详情与编辑者预览回归（2026-10-12）
- 匿名访问时，未发布、无发布时间或未来发布时间的页面不会出现在公开列表、详情 API、服务端 HTML 或 sitemap 中；详情请求返回 404。已授权编辑者仍可预览草稿和未来页面，预览响应标记 `noindex, nofollow`。
- 本轮 Windows 本地复验：`go test ./internal/httpapi ./internal/service ./internal/store` 通过；在 `web/` 运行 `npm run test:e2e`，25/25 流程通过，包含草稿隐私与授权预览。

## 备份响应超时（2026-10-12）
- HTTP 写超时现在通过 `APP_WRITE_TIMEOUT` 配置，默认 `5m`，并应用于快照生成、ZIP 压缩和完整下载；部署说明要求将应用与反向代理时限按最大备份体积共同设置。
- 配置回归覆盖默认值、有效时长和非正时长回退；Windows 本地复验 `go test ./...`、`go vet ./...` 和 `git diff --check` 通过。上一轮已通过的完整 Playwright E2E 为 25/25。
- `APP_SESSION_TTL_MIN` 现在拒绝超出 `time.Duration` 表示范围的正整数，并在解析层和会话计算层回退到 1440 分钟；配置与 HTTP 回归覆盖边界。当前 `go test ./...`、`go vet ./...` 及包含 `internal/config` 的目标包 race 检查通过。

## 管理端文章状态筛选分页（2026-10-12）
- 文章管理列表的草稿/已发布筛选现在由 API 与 SQLite 在分页前完成，筛选后的 `total` 和页数对应当前状态。`status=draft` 需要内容编辑权限；非法状态返回 422。
- 回归覆盖公开文章结果、匿名草稿筛选拒绝、编辑者分页结果和非法状态；Playwright 创建 101 篇草稿，检查筛选总数并读取第二页，完整 E2E 25/25 通过。
- Windows 本地验证：`go test ./...`、`go vet ./...`、`go test -race ./internal/config ./internal/service ./internal/store ./internal/httpapi` 和 `git diff --check` 通过。

## 文章列表分类/标签批量读取（2026-10-12）
- 搜索结果、管理列表和全量仓储读取现在按不超过 200 篇一批取回文章的分类/标签关联及公开文章计数，消除逐篇查询关联造成的 N+1 数据库往返。
- `TestPostListBatchPopulatesTermsAndVisibleCounts` 覆盖公开文章、排期文章和草稿的关联与公开计数；本轮全量 Go 测试、`go vet`、目标包 race 检查及 Playwright E2E 25/25 通过。

## 读者端分页加载 E2E 等待（2026-10-12）
- 导航页面和分类筛选的 E2E 在每次点“加载更多”后等待下一条链接出现，再触发下一批，避免连续请求撞上禁用中的加载按钮；页面详情流程同时断言当前页面链接选中，文章列表链接不误选。
- Windows 本地复验：`npm run test:e2e` 通过，25/25 流程通过，包含 TypeScript 检查与 Web 生产构建；`git diff --check` 通过。
- `APP_COOKIE_SECURE` 的配置回归确认只有精确值 `true` 会开启 Secure cookie；`TestSecureCookieSettingAppliesToCSRFAndSessionCookies` 通过 TLS HTTP 响应验证 CSRF cookie 保持脚本可读、会话 cookie 保持 HttpOnly，二者都带 Secure。随后 `go test ./...`、`go vet ./...` 与 `git diff --check` 通过。

## SQLite 唯一约束与 API 冲突响应（2026-10-12）
- 仓储层按 SQLite 扩展约束码将唯一键/主键冲突转为 `store.ErrConflict`；HTTP 为文章、页面、词条和用户名返回各自稳定的 409 错误码，不再依赖数据库错误文本。
- 媒体 multipart 请求超限通过 `errors.As` 识别 `http.MaxBytesError`，不再比对错误文本；`TestOversizedMultipartUsesTypedBodyLimit` 验证真实超限请求返回 413 `file_too_large`。
- `TestSQLiteUniqueConstraintsExposeConflict` 验证唯一约束归类和 CHECK 约束区分；`TestUniqueConflictsUseStableResourceErrors` 验证四类 API 响应。本轮 `go test ./...`、`go vet ./...`、目标包 race 检查及 Playwright E2E 25/25 通过。

## HTTP API 私密字段边界复核（2026-10-13）

- 按 API 审查清单重新核对响应字段、授权边界和客户端调用后，发现匿名 `GET /api/v1/system/status` 复用了完整站点设置对象，会返回管理邮箱。公开状态现只返回读者端需要的站点信息；完整站点设置通过 owner-only 的 `GET /api/v1/settings/site` 获取，设置页登录后再加载该数据。
- 仓储模型中的站点管理邮箱、用户密码哈希、评论邮箱和媒体存储键默认不参与 JSON 编码；需要返回的 owner 字段仍由显式 API 响应映射提供。
- 回归覆盖匿名接口不含管理邮箱、owner 接口可读管理邮箱、仓储敏感字段默认隐藏，以及浏览器设置页加载邮箱。Windows 验证：`go test ./...`、`go vet ./...`、`go test -race ./internal/config ./internal/service ./internal/store ./internal/httpapi` 和 Playwright E2E 25/25 通过。
