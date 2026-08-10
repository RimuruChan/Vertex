# Vertex Web UI

`ui` 是 Vertex 的 React 单页应用，提供题目、提交、比赛、榜单、题解、讨论与管理员界面。HTTP 请求类型和函数由后端 OpenAPI 规范通过 Orval 生成。

## 技术栈

- React 19、TypeScript、Vite 6。
- Tailwind CSS 4 + Radix UI(shadcn 风格的自有组件,位于 `src/components/ui/`)。
- CodeMirror、React Router、lucide-react 图标。
- React Markdown、KaTeX、DOMPurify。
- Axios + Orval 生成客户端。
- pnpm 11.9.0。

设计令牌(颜色、圆角、字体、判定色板)集中定义在 `src/index.css`,浅色与深色两套值都在那里;
`ThemeProvider` 只负责在 `<html>` 上切换 `.dark`。新增组件请使用令牌类(`bg-card`、
`text-muted-foreground`、`bg-verdict-ac-bg` 等),不要写死颜色。

## 环境要求

- Node.js 22。
- pnpm 11.9.0（版本记录在 `package.json`）。
- 默认开发代理期望 Vertex Server 运行在 <http://localhost:8080>。

## 本地开发

```bash
pnpm install --frozen-lockfile
pnpm run dev
```

打开 <http://localhost:5173>。Vite 会把 `/api` 请求代理到本地 Server。

常用命令：

| 命令 | 用途 |
|---|---|
| `pnpm run dev` | 启动 Vite 开发服务器 |
| `pnpm run build` | TypeScript 检查并生成生产构建 |
| `pnpm run preview` | 本地预览 `dist/` |
| `pnpm run api:generate` | 从 Web OpenAPI 规范重新生成客户端 |
| `pnpm run api:check` | 重新生成并检查产物是否有未提交差异 |

## API 客户端生成

API 规范来源是 `../server/docs/swagger.json`。当 Server handler、路由或 DTO 变化时，从仓库根目录依次运行：

```bash
go -C server generate .
pnpm --dir ui run api:generate
```

`src/generated/api/` 完全由 Orval 管理，不应手工编辑。自定义 Axios 行为位于 `src/api/http.ts`，负责 credentials、Authorization 和 access token 刷新；生成配置位于 `orval.config.ts`。

## 生产构建

```bash
pnpm install --frozen-lockfile
pnpm run build
```

静态文件输出到 `dist/`。生产环境应由 nginx 或其他静态服务器提供文件，并将 `/api/` 反向代理到 Vertex Server。仓库的 `deploy/nginx.conf` 可作为起点。

## 目录结构

```text
src/pages/          页面组件
src/pages/admin/    管理员页面
src/components/     业务组件(判定标签、题面渲染、代码编辑器、分栏布局等)
src/components/ui/  基础 UI 原语(Button/Card/Table/Dialog/Select/Toast…)
src/hooks/          共享 hook(如提交轮询)
src/lib/            cn() 与展示格式化工具
src/auth/           会话状态与启动恢复
src/api/            手写 HTTP 适配层
src/generated/api/  Orval 生成的 API 函数和 DTO
src/RootRoutes.tsx  路由定义
src/App.tsx         应用级布局与 provider
```
