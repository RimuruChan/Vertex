# 社区与后台管理

本页说明题单、题解、讨论三块社区功能,以及站点后台的能力边界。

## 1. 题单(Problem sets)

题单是**策展产物**,不是容器:把题目移出题单不会影响题目本身、也不会影响任何提交;
同一道题可以同时出现在多个题单里。

| | |
|---|---|
| 谁能建 | 具有当前域 `problem_set.create` 能力的有效成员（预置 member/author/admin） |
| 谁能改 | owner、域资源管理者与 editor 协作者；改变可见性、删除、转让及授权仅 owner/域资源管理者 |
| 可见性 | `public` / `private`；私有题单只对 owner、有效用户/group reader/editor 与域资源管理者开放 |
| 顺序 | 由策展人决定,保存时按数组顺序写入 `sort_order` |
| 备注 | 每题一条 `note`,用来写「先做这题」之类的提示 |
| 上限 | 单个题单 500 题 |

**进度**是读模型:题单列表与详情都会带上「当前查看者通过了其中几题」,
由 `submissions` 现算,不存冗余计数,因此重测、改判都不会让进度失真。
匿名访问时进度恒为 0,并且不会去查 `submissions`。

**未公开的题目**：引用只能在同域内进行，且编辑者需要当前题目访问权限（owner、题目 reader/editor 或域资源管理权）。题单授权不包含题目授权，条目、计数与进度都会过滤不可见题目。若编辑者看不到部分已有条目，整单替换会被拒绝，以免保存不完整视图时误删隐藏条目；标题、简介仍可独立保存。

创建者 `author_id` 仅作归属记录，转让只改变 `owner_id`。协作者可以查看授权列表，只有 owner/域资源管理者可以添加或移除授权。移除个人授权不会取消仍有效的 group 继承。停用成员立即失去所有权带来的操作权限；归档域只读。前端所有管理动作位于详情页。

## 2. 题解(Editorials)

题解先继承当前域与父题目的访问边界，再判断作者、内容可见性和防剧透设置。曾经写过题解不等于永久获得父题目的访问权。

- **编辑与删除**：有效域成员可编辑、删除自己仍有权访问的内容；域资源管理者与父题目 owner 可在可见范围内治理删除，但不能以作者名义改写内容。停用成员失去操作权；归档域只读。
- **点赞**:`editorial_votes` 一人一票,冗余计数 `vote_count` 每次由投票表重算,
  重复点击或重试都不会让计数漂移。列表可按 `sort=votes` 排序。
- **草稿与私有内容**：仅有效作者及域资源管理者可见，父题目 owner 不因此自动看到他人的私有草稿。
- **防剧透(`solved_only`)**:打开后,**没通过该题的读者只能看到标题**,正文被服务端摘掉,
  并带上 `locked` 标记让前端解释原因。
  - 条目本身仍然可见——否则读者根本不知道题解存在。
  - 已通过父资源检查的作者与相应治理者不受防剧透限制。
  - 判定依据是当前是否存在练习 Accepted 提交，不用比赛提交解锁。列表无正文；详情在 SQL 投影中屏蔽正文。未解锁时不能点赞、读取讨论或回复；重测失去 AC 后重新锁定。

## 3. 讨论(Discussions)

- 两种作用域：题目、题解。比赛不使用普通讨论；交流统一使用澄清接口，旧比赛讨论接口及 schema 字段已移除。
- 两种讨论都支持楼中楼；父回复必须属于同一道题或同一篇题解，事务检查和复合 FK 共同约束，不能只验证属于同域。
- **编辑自己的楼**:`updated_at` 与 `created_at` 拉开距离即视为「已编辑」,前端据此标注。
- **删除**：在父资源授权范围内，作者或相应治理者可删除；删除父楼会级联删除回复并记录审计。题解作者可以治理自己可见题解下的讨论，但不能改写评论。
- 返回 `permissions` 和线程 `canPost`，客户端不自行用站点角色推导编辑/删除/发言权。发新内容需要域 `content.create` 能力，已有内容的作者编辑权不依赖是否仍可发新内容。
- 正文上限 20000 字符。

## 4. 站点后台

`/admin` 只保留站点状态与账号治理，需要站点管理员权限；域内容不在这里跨域编辑。读取错误显示可重试状态，不以 0 或无限加载冒充成功。

### 概览

站点计数(用户/题目/提交/比赛/题解/题单,以及当日新增)与**判题队列健康度**:
排队数、判题中、已放弃、活跃 worker 数、最早排队时间。
「有排队但活跃 worker 为 0」是运维第一眼要看的信号,控制台直接把它标红。
另有最近 24 小时的判定分布。

### 用户

按用户名/邮箱搜索,按角色过滤,可改角色与 rating,可封禁/解封。

**封禁语义**:不删账号——提交、榜单、题解都要留下,只是不能再登录。

- 封禁时**同时吊销该账号的全部会话**,所以是立即生效,而不是等 refresh 窗口过期。
- 登录、刷新与 access token 校验三条路径**各自**检查 `disabled_at`,
  即使有人直接改数据库绕过控制台,封禁依然有效。
- 密码错误时仍然返回「凭据错误」而不是「账号被封」——否则等于向不知道密码的人确认账号存在。
- 管理员**不能封禁自己、也不能取消自己的管理员权限**:这两件事会把安装锁在门外。

## 5. 域资源治理

域 owner 与具有 `domain.resources.manage` 的有效成员可以维护本域标签与公告，不需要站点管理员角色。路由先检查治理能力再读取修改请求体，持久层在事务内锁定账号/域并重新授权；待锁期间发生的停用或撤权同样生效。归档域允许有权管理的人只读查看，禁止写入。相关操作写入域审计，不记录公告正文或凭据。

### 标签

标签目录带题目计数,可重命名、合并、删除。
**重命名成一个已存在的名字会自动合并**——这正是发现重复标签(`dp` / `DP`)时想做的事。
合并会把源标签的题目全部转到目标标签(已同时拥有两者的题目只保留一条关联),再删掉源标签。

`/d/{domain}/settings/tags` 只查找、创建和进入；详情负责重命名、合并与删除。标签变更影响当前题库分类，同时同步可变工作副本的标签并增加 package revision，避免下一次发布恢复旧名字；data revision 不变，不强制重建评测材料。已发布版本的 `tags_json` 保持原样，比赛固定版本和跨域复制仍使用其历史快照。目录治理与题目写入/发布共用按域的互斥 guard，先取得目录 guard，再取得具体题目的锁，避免在发布中途改写分类。

管理目录的计数包含本域所有关联题目；公开 `/tags` 则按当前查看者的题目权限过滤计数。删除标签不删除题目，但会移除当前分类与工作副本标签，因此需确认影响范围。

### 公告

公告属于域，支持置顶与草稿。`/d/{domain}/settings/announcements` 提供搜索、分页和创建，创建界面先保存草稿，再进入数字编号详情编写正文、预览和明确发布。列表/计数在数据库中先按域与公开状态过滤；公开接口始终只返回已发布公告，即使调用者是站点管理员或域 owner 也不自动混入草稿。草稿从治理接口读取。

公开地址为 `/d/{domain}/announcements/{number}`，域内数字编号稳定且删除后不复用，内部保留 UUID。首页和页脚提供公开入口。切换域只保留公告栏目，不将原公告号带入另一域；未发布或已删除公告返回明确错误。

## 6. 相关接口

下面的旧无域资源接口固定官方域；正常页面使用 `/api/domains/{domain}/...`。账号与系统统计仍为全站接口。

```text
GET    /api/problem-sets                     题单列表(带进度)
GET    /api/problem-sets/{id}                题单详情
POST   /api/problem-sets                     新建(当前域创建能力)
PUT    /api/problem-sets/{id}                改标题/简介/可见性
PUT    /api/problem-sets/{id}/items          整体替换题目与顺序
DELETE /api/problem-sets/{id}
GET    /api/problem-sets/{id}/access         直接用户/group 授权
PUT    /api/problem-sets/{id}/access         设置 reader/editor
DELETE /api/problem-sets/{id}/access/{grantId}
PUT    /api/problem-sets/{id}/owner          转让给有效域成员

GET    /api/editorials?problem=&sort=votes   题解列表(应用防剧透)
PUT    /api/editorials/{id}                  作者编辑
DELETE /api/editorials/{id}                  作者或管理员删除
POST   /api/editorials/{id}/vote             点赞 / 取消

GET    /api/problems/{id}/discussions        题目讨论及 canPost
POST   /api/problems/{id}/discussions        发表 / 回复
GET    /api/editorials/{id}/discussions      题解讨论及 canPost
POST   /api/editorials/{id}/discussions      发表 / 回复
PUT    /api/discussions/{postId}             编辑自己的楼
DELETE /api/discussions/{postId}             作者或管理员删除

GET    /api/announcements                    仅已发布公告，支持 page/size/keyword
GET    /api/announcements/{id}               公开公告详情
GET    /api/admin/stats                      站点概览与队列健康度
GET    /api/admin/users                      用户检索
PATCH  /api/admin/users/{id}                 角色 / rating / 封禁
GET    /api/admin/tags                       标签目录(带题目数)
POST   /api/admin/tags                       创建本域标签
GET    /api/admin/tags/{id}                  标签详情
PUT    /api/admin/tags/{id}                  重命名(同名即合并)
POST   /api/admin/tags/{id}/merge            合并到指定标签
DELETE /api/admin/tags/{id}
GET    /api/admin/announcements              本域治理列表，包含草稿
GET    /api/admin/announcements/{id}          本域治理详情
POST   /api/admin/announcements              创建公告
PUT    /api/admin/announcements/{id}          保存内容及发布状态
DELETE /api/admin/announcements/{id}          删除公告
```

## 7. 已知边界

- **举报与审核队列**:管理员可以删除内容,但没有用户举报入口和待审队列。
- **题解版本历史**:编辑会覆盖旧内容,不保留修订记录。
- **题单收藏/分叉**:没有「收藏他人题单」或「复制成自己的」。
- **富文本附件**:题解与公告都只有 Markdown,不支持上传图片。
- **密码重置**:管理员可以封禁账号,但不能替用户重置密码。
