# SourceBox

自托管的 AltStore / SideStore 源生成器：读取一组 GitHub 仓库的最新 Release，把其中的 `.ipa` 资源汇总成一个 `source.json`，并提供一个可管理的 Web 界面。

它是 [GithubStore](https://github.com/yazdipour/GithubStore) 的重写版，重点在界面、状态可见性和运维体验：

- **仪表盘**：源地址票券 + 二维码、仓库卡片、版本/体积/刷新时间、状态徽章
- **仓库管理**：网页添加/删除、单独刷新、全部刷新、重命名、强调色、图标覆盖
- **错误可见**：GitHub 限流、404、DMCA 封禁、无 Release、Release 无 IPA 都区分成独立状态
- **图标与二维码**：图标代理并落盘缓存，缺图标时按强调色生成占位图标
- **轻量单容器**：Go 单二进制 + SQLite，前端构建后由同一个进程托管，无运行时依赖

## 快速开始

```bash
cp .env.example .env      # 按需填写 BASE_URL / ADMIN_TOKEN / GITHUB_TOKEN
docker compose up -d --build
```

容器起来后看日志里的后台地址（形如 `http://localhost:8080/p-8hhkntcjt4scm68a/`）：

1. 点 **添加仓库**，粘贴形如 `https://github.com/owner/repo` 的地址
2. SourceBox 会立刻拉取最新 Release，并显示是否命中 `.ipa`
3. 把票券里的地址（如 `http://localhost:8080/p-8hhkntcjt4scm68a/source.json`）加到 SideStore / AltStore

## 环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `BASE_URL` | `http://localhost:8080` | 对外地址，会写进 `source.json` 的图标与下载链接 |
| `ADMIN_PATH` | 自动生成 | 后台入口路径段。留空则首次启动随机生成并存入数据库，可用 `./sourcebox -print-admin-path` 取回 |
| `ADMIN_TOKEN` | 空 | 设置后仪表盘需要登录；留空则任何人可管理 |
| `GITHUB_TOKEN` | 空 | 可选。GitHub API 限额从 60 次/小时提升到 5000 次/小时 |
| `CACHE_SECONDS` | `600` | Release 数据缓存时长，可在设置页覆盖 |
| `DATA_DIR` | `./data` | SQLite 与图标缓存目录 |
| `PORT` | `8080` | 监听端口 |

> 源的 JSON、图标和后台页面共用同一段随机路径：`/<ADMIN_PATH>/source.json`、`/<ADMIN_PATH>/<slug>.json`、`/<ADMIN_PATH>/icons/*`、`/<ADMIN_PATH>/source-icon.png`。根路径 `/`、`/source.json`、`/api/*`、`/icons/*` 一律 404，未认证的只有 `/health`（不含任何地址信息）。

## 安全性

**后台和源都在同一段随机路径下。** 首次启动会生成 `p-` + 16 位随机字符并存进数据库，例如 `https://store.example.com/p-8hhkntcjt4scm68a/`；源地址则是 `https://store.example.com/p-8hhkntcjt4scm68a/source.json`。`/`、`/source.json`、`/api/*`、`/icons/*` 全部返回 404，扫描器既拿不到登录页，也拿不到源。路径可用 `ADMIN_PATH` 固定，或用下面的命令取回：

```bash
./sourcebox -print-admin-path
```

**GitHub token 不会离开服务端。** 前端只拿到 `githubTokenConfigured: true/false` 这个布尔值，用来显示「已配置 / 未配置」；token 只出现在后端发往 `api.github.com` 的 `Authorization` 请求头里，任何接口、页面、`source.json` 都不包含它。仓库出错时回给前端的是分类后的提示（限流 / 404 / DMCA / 无 IPA），不会回显内部异常堆栈。

**服务器 IP 只在你写了的地方出现。** `source.json` 里的图标与下载地址由 `BASE_URL` 拼接；把它设成域名（如 `https://store.example.com`）就不会出现 IP。配合 Cloudflare Tunnel 时源站 IP 完全不暴露，也不需要开放任何入站端口。

**其余加固：**

- 登录失败 8 次后锁定该来源 IP 15 分钟；token 比较使用常量时间
- 会话写入 `HttpOnly` + `SameSite=Lax` cookie，HTTPS 下自动加 `Secure`；不写 localStorage
- 管理接口要求 `Content-Type: application/json`，配合 `SameSite=Lax` 阻断跨站表单提交
- 页面带 CSP（`script-src 'self'`）、`X-Frame-Options: DENY`、`Referrer-Policy: no-referrer`、`X-Content-Type-Options: nosniff`
- 图标代理会拒绝回环 / 私有网段地址，避免被当成 SSRF 跳板
- 静态文件路径做 Clean + 目录约束，`../` 穿越返回 404

> 随机路径是「降低暴露面」，不是认证。**务必同时设置 `ADMIN_TOKEN`**，它是唯一真正的访问控制。两者都在时才安全。

## 本地开发

```bash
# 后端（Go 1.24+）
go mod download
go run .            # http://localhost:8080

# 前端（另开终端）
cd web
npm install
npm run dev        # http://localhost:5173，/api 会代理到 8080
```

生产构建：

```bash
cd web && npm run build   # 产物在 web/dist，后端启动时自动托管
go test ./...             # 单元 + 接口测试
go vet ./...
cd web && npm run typecheck   # 可选：完整类型检查
```

## 目录结构

```text
main.go                    进程入口：配置、SQLite、HTTP 服务、优雅退出
internal/config            环境变量
internal/store             SQLite 层（仓库、KV、缓存 JSON）
internal/github            GitHub REST 客户端（404 / 451 / 限流分类）
internal/source            .ipa 挑选、app 条目、source.json 文档
internal/refresh           抓取编排、TTL 判断、并发刷新
internal/icons             占位图标生成、远程图标缓存、二维码 SVG
internal/legacy            首次启动导入 GithubStore 的 config.yml
internal/httpapi           路由、鉴权、静态托管
web                         Vue 3 + TS 前端
```

## 接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/<ADMIN_PATH>/source.json` | 生成的 AltStore 源（公开） |
| GET | `/<ADMIN_PATH>/<slug>.json` | 同上的别名（公开） |
| GET | `/<ADMIN_PATH>/icons/{slug}.png` | 仓库图标（公开，代理 + 缓存） |
| GET | `/health` | 健康检查（公开） |
| GET | `/api/me` | 登录状态 |
| POST | `/api/login` / `/api/logout` | 管理员会话 |
| GET | `/api/overview` | 仪表盘数据 |
| POST | `/api/repos` | 添加仓库并立即抓取 |
| PATCH | `/api/repos/{id}` | 改名 / 强调色 / 图标 |
| DELETE | `/api/repos/{id}` | 移除仓库 |
| POST | `/api/repos/{id}/refresh` | 刷新单个仓库 |
| POST | `/api/repos/refresh` | 刷新全部 |
| GET/PUT | `/api/source` | 源名称、slug、副标题、描述、强调色、缓存时长 |
| GET | `/api/preview` | 不触发抓取的源预览 |

## 从 GithubStore 迁移

首次启动时，如果数据库为空，SourceBox 会尝试读取 `config.yml`（`SEED_CONFIG` 指定路径、`DATA_DIR/config.yml`、`/app/config.yml`），导入其中的 `source.*` 元数据和 `repositories` 列表。之后以 SQLite 为准，`config.yml` 不再被写入。

## 部署在反向代理 / Cloudflare Tunnel 后面

把 `BASE_URL` 设成最终对外地址（例如 `https://store.example.com`），再重启容器即可：

```yaml
# docker-compose.yml 片段
environment:
  BASE_URL: https://store.example.com
  ADMIN_TOKEN: ${ADMIN_TOKEN}
```

Cloudflare Tunnel 只需一条 ingress 指向 `http://127.0.0.1:8080`，不需要开放入站端口：

```yaml
ingress:
  - hostname: store.example.com
    service: http://127.0.0.1:8080
  - service: http_status:404
```

`BASE_URL` 会决定 `source.json` 里的图标地址与下载链接，填错会导致客户端拿不到图标。

## 设计说明

界面刻意避开通用深色后台模板：

- 签名元素是**源地址票券**：打孔缺口、虚线撕线、右侧二维码，直接对应「把地址兑换到手机里」这个动作
- 仓库卡片用各自的强调色做 1px 描边，图标压边，状态用等宽字体徽章
- 字体走系统 UI 栈 + 等宽栈，靠字重、字距和表格数字建立精密感，不依赖外部字体 CDN
- 深色为默认，可切换浅色，并尊重 `prefers-reduced-motion`

## 说明

- 只读取 GitHub 的 **latest release**，不跟踪预发布或历史版本
- 一个 Release 里多个 `.ipa` 会拆成多个 app 条目，普通构建优先、体积大的优先
- MIT License，见 [LICENSE](LICENSE)

