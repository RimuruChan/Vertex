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

在原有「发布 + 列表」的基础上补齐了作者与读者两侧:

- **编辑与删除**:作者可改可删;管理员可删(作为内容治理),但**不能改**——
  以他人名义改写内容比删除更糟。
- **点赞**:`editorial_votes` 一人一票,冗余计数 `vote_count` 每次由投票表重算,
  重复点击或重试都不会让计数漂移。列表可按 `sort=votes` 排序。
- **草稿**:`status = draft` 的题解只有作者自己看得到。
- **防剧透(`solved_only`)**:打开后,**没通过该题的读者只能看到标题**,正文被服务端摘掉,
  并带上 `locked` 标记让前端解释原因。
  - 条目本身仍然可见——否则读者根本不知道题解存在。
  - 作者与管理员不受限制。
  - 判定依据是「是否有过 Accepted 提交」,在服务层完成,前端拿不到被屏蔽的正文。

## 3. 讨论(Discussions)

- 三种作用域:题目、题解、比赛(`contest_id` 从第一版就在表里,这次接上了读写路径)。
- **编辑自己的楼**:`updated_at` 与 `created_at` 拉开距离即视为「已编辑」,前端据此标注。
- **删除**:作者或管理员;删除父楼时回复通过外键级联一起删除,不留孤儿回复。
- 正文上限 20000 字符。

## 4. 站点后台

`/admin` 是一个独立的管理控制台,分四块。

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

### 标签

标签目录带题目计数,可重命名、合并、删除。
**重命名成一个已存在的名字会自动合并**——这正是发现重复标签(`dp` / `DP`)时想做的事。
合并会把源标签的题目全部转到目标标签(已同时拥有两者的题目只保留一条关联),再删掉源标签。

### 公告

站点公告支持置顶与草稿。已发布的公告出现在首页顶部与公开接口 `/api/announcements`;
草稿只有管理员看得到。

## 5. 相关接口

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

GET    /api/contests/{id}/discussions        比赛讨论
PUT    /api/discussions/{postId}             编辑自己的楼
DELETE /api/discussions/{postId}             作者或管理员删除

GET    /api/announcements                    公开公告(草稿仅管理员可见)
GET    /api/admin/stats                      站点概览与队列健康度
GET    /api/admin/users                      用户检索
PATCH  /api/admin/users/{id}                 角色 / rating / 封禁
GET    /api/admin/tags                       标签目录(带题目数)
PUT    /api/admin/tags/{id}                  重命名(同名即合并)
POST   /api/admin/tags/{id}/merge            合并到指定标签
DELETE /api/admin/tags/{id}
POST   /api/admin/announcements              发布公告
```

## 6. 已知边界

- **举报与审核队列**:管理员可以删除内容,但没有用户举报入口和待审队列。
- **题解版本历史**:编辑会覆盖旧内容,不保留修订记录。
- **题单收藏/分叉**:没有「收藏他人题单」或「复制成自己的」。
- **富文本附件**:题解与公告都只有 Markdown,不支持上传图片。
- **密码重置**:管理员可以封禁账号,但不能替用户重置密码。
