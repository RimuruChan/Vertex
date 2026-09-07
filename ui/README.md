# Vertex Web UI

`ui` 是 Vertex 的 React 单页应用，提供题目、提交、比赛、榜单、题解、讨论与管理员界面。HTTP 请求类型和函数由后端 OpenAPI 规范通过 Orval 生成。

## 技术栈

- React 19、TypeScript、Vite 6。
- Tailwind CSS 4 + Radix UI（自有基础组件位于 `src/components/ui/`）。
- CodeMirror、React Router、lucide-react 图标。
- markdown-it、KaTeX、DOMPurify。
- Axios + Orval 生成客户端。
- pnpm 11.9.0。

设计令牌（颜色、圆角、字体、判定色板）集中定义在 `src/index.css`，浅色与深色两套值都在那里；
`ThemeProvider` 只负责在 `<html>` 上切换 `.dark`。视觉方向为简约现代：中性底色、蓝色强调、
清晰的排版与轻边框。统一页头使用 `PageHeading`，内容容器使用 `page-shell` / `surface-panel`。
新增组件请使用令牌类（`bg-card`、`text-muted-foreground`、
`bg-verdict-ac-bg` 等），不要写死颜色。普通页面、文章阅读和做题工作台分别使用适合自身任务的
密度；手机和平板上的工作台使用单面板切换，而不是压缩桌面分栏。

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

## 无后端 mock 模式

在 `ui` 目录运行：

```bash
pnpm install --frozen-lockfile
pnpm run dev:mock
```

打开 <http://localhost:5173> 即可体验；不需要 Server、数据库、Worker 或 Docker。
默认以 `demo` 登录，退出后可使用 `demo / demo123`，登录页也能一键填入演示账号。
不支持注册真实账号，勿输入个人密码。

- 题库：搜索、标签/难度/进度筛选，题面和独立的代码草稿。
- 提交：排队、逐测试点进度、结果详情与列表筛选/分页；不编译或执行代码。
- 社区：题解与讨论继承父题目权限，作者编辑和治理删除分开；题目/题解均可线程回复，防剧透同时限制正文、点赞与讨论，mock 点赞按独立账号保存。
- 域：顶部在官方、算法训练营与命题小组之间切换。私域按身份过滤；同号资源使用独立 UUID 和数据图，旧浏览器数据保留在官方域。切换只保留栏目，不携带上一域题号、比赛号或筛选参数。
- 题单：创建、分别保存设置/编排、用户协作/转让和按权限过滤的进度展示；比赛提供创建/编排、报名、题面、示例榜单与澄清提问。资源授权的完整 group 继承 mock 仍待接入。
- 域治理：`/domains` 支持分页查找、创建、申请/加入和接受邀请；新建域是空空间。`/d/{domain}/settings` 提供设置、成员、角色与归档恢复，`groups` 列表进入数字编号详情维护组设置、成员及所有权。组管理者不获得域治理或组 owner 权限。
- 出题：登录后从正常导航进入当前域工作台，已有 owner/协作者不需要站点管理员角色。创建需要域权限；`demo` 在官方是普通成员，在训练域是出题人。列表仅搜索、创建和进入；编辑、上传与删除在详情。mock 模拟题面、文件、测试点、构建、ZIP 导入与显式发布，但不解析 ZIP 或运行代码。
- 身份：顶部设置统一切换访客、普通用户、选手、裁判、观察员、管理员。账号的报名与私有提交分开，赛务权限只在指定比赛生效；没有单独的出题演示切换页。
- 顶部「演示模式」可切换正常、慢速、空列表、加载失败，并指定下一次 AC / WA / TLE / CE。

演示数据以 `vertex-mock:v2` 保存于当前浏览器，兼容升级旧 v1 数据，刷新后保留。重置按钮清除这两代演示数据和
`vertex-mock-draft:*` 草稿，恢复初始内容；不清除正常模式的代码草稿。如果浏览器禁止持久化，
交互仍可在当前页面内运行。mock 展示当前接入的权限视角，但不能替代后端权限、跨域隔离和真实评测测试。
域创建、成员/角色/group 治理已使用正常页面与模拟接口；完整资源协作继承和复制的 mock 仍待补齐。未实现接口明确报错，不回退真实网络请求。切换演示身份只影响 mock 会话，不改变正式后端权限，也不把域角色写成账号的全局角色。

页面使用 `/d/{domain}/problems/{publicId}` 等域内数字地址；比赛题页为 `/d/{domain}/contests/{contest}/problems/{label}`，不挂载普通题解/讨论。内部请求体和代码草稿仍以 UUID 标识。旧 `/problems`、`/authoring` 等地址规范化为官方域。认证和站点后台分别保留 `/login`、`/admin`。

资源页面在 `DomainProvider` 下消费域信息，使用 `useDomainAPI` 绑定生成客户端的首个域参数，链接与导航来自 `domain/navigation`。每个函数捕获自己的域，不通过可变全局状态改写请求；切换域/账号时页面重新挂载并中止旧读取。登录返回地址保留域。域加载失败仍保留登录与 mock 设置，不把错误伪装成空域。

`.env.mock` 的 `VITE_MOCK=true` 只在显式选择 `mock` mode 时加载。请求仍经过同一份 Orval
客户端，在挂载应用、恢复登录前安装 Axios mock adapter；fixture 直接引用生成的 DTO 类型。
正常 `pnpm run dev` / `pnpm run build` 不加载演示数据。不要在生产环境设置 `VITE_MOCK=true`。

需要独立的静态演示构建时使用 `pnpm run build:mock`，然后 `pnpm run preview:mock`。该产物会明确
显示演示标记，输出到 `dist-mock/`，与正常的 `dist/` 分开。

常用命令：

| 命令 | 用途 |
|---|---|
| `pnpm run dev` | 启动 Vite 开发服务器 |
| `pnpm run dev:mock` | 启动无需后端的交互演示 |
| `pnpm run build` | TypeScript 检查并生成生产构建 |
| `pnpm run build:mock` | 生成独立静态演示构建（`dist-mock/`） |
| `pnpm run preview:mock` | 本地预览 `dist-mock/` |
| `pnpm run preview` | 本地预览 `dist/` |
| `pnpm run api:generate` | 从 Web OpenAPI 规范重新生成客户端 |
| `pnpm run api:check` | 重新生成并检查产物是否有未提交差异 |
| `pnpm run format:check` | 检查全部前端源码格式 |
| `pnpm run test` | 运行 Vitest 单元测试 |

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
pnpm run test
pnpm run build
```

静态文件输出到 `dist/`。生产环境应由 nginx 或其他静态服务器提供文件，并将 `/api/` 反向代理到 Vertex Server。仓库的 `deploy/nginx.conf` 可作为起点。

也可以从仓库根目录直接构建包含 UI 静态产物的 nginx 镜像并启动完整栈：

```bash
docker compose --profile with-frontend up -d --build --wait --wait-timeout 120
```

`ui/Dockerfile` 在独立构建阶段使用 `pnpm-lock.yaml` 安装依赖并生成 `dist`，最终镜像不依赖宿主预先构建或挂载静态文件。

## 目录结构

```text
src/pages/          页面组件（按路由懒加载）
src/pages/admin/    管理员页面
src/components/     业务组件(判定标签、题面渲染、代码编辑器、分栏布局等)
src/components/ui/  基础 UI 原语(Button/Card/Table/Dialog/Select/Toast…)
src/hooks/          共享 hook(如提交轮询)
src/lib/            cn() 与展示格式化工具
src/auth/           会话状态与启动恢复
src/api/            手写 HTTP 适配层
src/mocks/          演示 fixtures、状态 API、Axios adapter 和演示设置
src/generated/api/  Orval 生成的 API 函数和 DTO
src/RootRoutes.tsx  路由定义
src/App.tsx         应用级布局与 provider
```
