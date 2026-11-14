# Retro BBS — Project Plan & Technical Design

Design v1.0PlanningSmall Software

项目定位： 一个面向人类社区的传统文本 BBS，同时为 AI Agent 提供只读的历史讨论和资料访问能力。

本设计遵循你提出的 Small Software 原则：先解决当前需求，使用最简单的正确实现，不提前引入抽象层，不为了未来可能的需求增加复杂性。

# 1. 项目目标

## 1.1 核心目标

建立一个可以长期运行、容易部署、容易维护的文本 BBS。

支持三种使用场景：

| 使用者 | 访问方式 | 主要用途 |
| --- | --- | --- |
| Web 用户 | Browser + xterm.js | 浏览和参与社区 |
| Terminal 用户 | Go CLI + Bubble Tea | 原生终端体验 |
| AI Agent | 未来 MCP | 搜索和读取讨论 |

三种使用场景共享同一份数据和业务逻辑。

## 1.2 产品原则

- BBS 首先是一个社区，不是 Wiki。
- 主题帖和回复是核心内容。
- 每篇帖子都有明确的作者和发布时间。
- 帖子默认长期保留。
- 普通用户可以编辑自己的帖子，但不能删除。
- Markdown 是内容格式，不是 Wiki 功能。
- AI Agent 是只读访问者，不改变社区的基本形态。

## 1.3 非目标

第一版明确不实现：

- Wiki 页面及双向链接。
- REST API。
- 即时聊天。
- 实时协同编辑。
- 复杂 ANSI 绘图。
- Telnet Gateway。
- DOS 客户端。
- MCP Server。
- 微服务和分布式部署。
- Elasticsearch 等独立搜索服务。

# 2. 技术架构

## 2.1 总体架构

## Retro BBS Architecture

Web Client

HTML / CSS / JS

xterm.js

Go CLI

Bubble Tea

WebSocket Client

WebSocket + JSON

Go BBS Server

# main.go

HTTP · WebSocket · Authentication · BBS Logic

SQLite

Users / Profiles / Boards / Indexes

Filesystem

Posts / Drafts / Avatars

## 2.2 技术选型

| 技术 | 用途 | 选择理由 |
| --- | --- | --- |
| Go | Server | 单二进制发布、标准库丰富 |
| SQLite | Metadata | 不需要独立数据库服务 |
| Markdown | 帖子正文 | 易于阅读、迁移和备份 |
| xterm.js | Web Terminal | 成熟的终端模拟器 |
| Bubble Tea | Go TUI | 原生终端交互 |
| WebSocket | 通信 | Web 和 CLI 共用 |
| JSON | 消息格式 | 易于调试和扩展 |

### 第三方依赖

仅在有明确用途时引入：

| 依赖 | 用途 |
| --- | --- |
| `github.com/gorilla/websocket` | WebSocket |
| `modernc.org/sqlite` | SQLite，无需 CGO |
| `github.com/charmbracelet/bubbletea` | Go TUI |
| `github.com/yuin/goldmark` | Markdown 解析与渲染 |
| `golang.org/x/crypto` | Argon2id 密码哈希 |

Web 前端使用 xterm.js，建议固定版本并将文件本地部署。

不引入完整 Web Framework。

# 3. 数据设计

这是整个项目最重要的部分之一。

我们的原则是：

SQLite 管理结构化信息，文件系统保存正文。

## 3.1 SQLite Schema

第一版只需要以下几个核心表。

### users

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | TEXT | 用户 ID |
| username | TEXT | 登录名，唯一 |
| password\_hash | TEXT | 密码哈希 |
| created\_at | INTEGER | 创建时间 |
| status | TEXT | active / suspended |

### profiles

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| user\_id | TEXT | 用户 ID |
| nickname | TEXT | 显示名称 |
| bio | TEXT | 个人简介 |
| avatar | TEXT | 头像文件路径 |

### boards

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | TEXT | 版面 ID |
| name | TEXT | 版面名称 |
| description | TEXT | 版面说明 |
| created\_at | INTEGER | 创建时间 |

### threads

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | TEXT | 主题 ID |
| board\_id | TEXT | 所属版面 |
| author\_id | TEXT | 作者 |
| title | TEXT | 标题 |
| file\_path | TEXT | Markdown 文件路径 |
| created\_at | INTEGER | 创建时间 |
| updated\_at | INTEGER | 编辑时间 |
| reply\_count | INTEGER | 回复数量 |
| hidden | INTEGER | 管理员隐藏标记 |

### posts

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | TEXT | 回复 ID |
| thread\_id | TEXT | 所属主题 |
| author\_id | TEXT | 作者 |
| file\_path | TEXT | Markdown 文件路径 |
| created\_at | INTEGER | 创建时间 |
| updated\_at | INTEGER | 编辑时间 |

这里将主题帖和回复分开存储。

每篇主题和回复都有自己的 Markdown 文件。

### drafts

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | TEXT | 草稿 ID |
| user\_id | TEXT | 作者 |
| board\_id | TEXT | 目标版面 |
| thread\_id | TEXT | 可选，回复草稿 |
| file\_path | TEXT | 草稿文件 |
| updated\_at | INTEGER | 更新时间 |

## 3.2 文件存储

text

```text
data/
│
├── bbs.db
│
├── posts/
│   └── 2026/
│       └── 11/
│           ├── thread-001.md
│           ├── post-001.md
│           └── post-002.md
│
├── drafts/
│   └── user-001/
│       ├── draft-001.md
│       └── draft-002.md
│
├── avatars/
│   ├── user-001.webp
│   └── user-002.webp
│
└── tmp/
```

建议使用 UUID 或 ULID 作为 ID，避免依赖时间生成的 ID 发生碰撞。

文件路径由服务器根据 ID 生成，不能直接使用用户输入的文件路径。

## 3.3 Markdown 格式

正文文件只保存正文，不重复保存 SQLite 中已有的元数据。

例如：

markdown

```markdown
Hello everyone!

This is my first post.

I miss the old Internet.

I hope this BBS will become
a nice little community.
```

标题、作者、时间和版面信息都从 SQLite 获取。

这样可以避免元数据不一致。

## 3.4 数据一致性

文件和 SQLite 不是同一个事务系统，因此需要简单的写入顺序：

1. 检查用户权限和正文大小。
2. 将内容写入临时文件。
3. 同一文件系统内原子重命名。
4. 更新 SQLite 记录。
5. 如果数据库更新失败，保留或清理临时状态，并记录错误。

正式发布时，应该避免出现数据库记录指向不存在的文件。

第一版不需要复杂的事件日志，但需要一个简单的启动时一致性检查。

# 4. 帖子规则

## 4.1 帖子限制

配置文件：

toml

```toml
[posts]
max_size = 65536

[drafts]
max_size = 65536
```

单位：bytes。

| 项目 | 默认限制 |
| --- | --- |
| 主题帖 | 64 KiB |
| 回复 | 64 KiB |
| 草稿 | 64 KiB |
| 标题 | 200 bytes |
| 用户简介 | 2 KiB |
| 头像 | 512 KiB |

所有限制均可通过配置调整。

降低限制不会修改已有内容，只影响新建和编辑操作。

## 4.2 帖子生命周期

Draft

编辑和保存

Published

正常显示

Edited

作者可以修改正文

Hidden

仅管理员可以隐藏

第一版不提供普通用户删除帖子功能。

编辑帖子时更新 `updated_at`，不实现完整版本历史。

# 5. WebSocket 协议设计

WebSocket 是 Web 和 Go CLI 唯一的客户端通信协议。

不提供 REST API。

## 5.1 消息结构

客户端请求：

json

```json
{
  "id": 1001,
  "type": "thread_list",
  "data": {
    "board_id": "general",
    "page": 1
  }
}
```

服务器响应：

json

```json
{
  "id": 1001,
  "type": "response",
  "ok": true,
  "data": {
    "threads": []
  }
}
```

错误响应：

json

```json
{
  "id": 1001,
  "type": "response",
  "ok": false,
  "error": {
    "code": "NOT_FOUND",
    "message": "Thread not found"
  }
}
```

## 5.2 第一版消息类型

| 消息类型 | 用途 |
| --- | --- |
| `register` | 用户注册 |
| `login` | 用户登录 |
| `logout` | 用户登出 |
| `board_list` | 获取版面 |
| `thread_list` | 获取主题列表 |
| `thread_read` | 阅读主题及回复 |
| `thread_create` | 创建主题 |
| `post_reply` | 回复主题 |
| `post_update` | 编辑自己的帖子 |
| `draft_list` | 获取草稿 |
| `draft_save` | 保存草稿 |
| `profile_get` | 获取个人资料 |
| `profile_update` | 修改个人资料 |
| `search` | 搜索帖子 |

第一版采用请求/响应模型。

暂时不实现实时聊天、推送通知和复杂订阅机制。

## 5.3 协议规则

- 一条请求对应一条响应。
- 使用请求 ID 匹配响应。
- JSON UTF-8 编码。
- 每条消息限制最大字节数。
- 服务端验证消息类型和字段。
- 所有写入操作必须验证身份和权限。
- WebSocket 必须检查 Origin。
- 认证 Session 由服务端管理。

密码只通过 HTTPS/WSS 传输。

# 6. 终端 UI 设计

## 6.1 设计原则

不实现复杂 ANSI 绘图。

允许：

- 普通文本。
- 换行。
- SGR 颜色。
- 粗体。
- 重置样式。

不实现：

- 光标任意定位。
- 复杂屏幕刷新。
- 动画。
- 终端私有控制序列。

## 6.2 Web 客户端

使用 xterm.js。

MuffinTerm • A Terminal for the Classic BBS Experience

Va online la prima Bbs: era come stare sui social network prima del Web - la Repubblica

ICM: "Interested in an authentic BBS experience? Check …" - Mastodon @ SDF

主要页面：

1. Welcome Screen
2. Login / Register
3. Board List
4. Thread List
5. Thread Reader
6. Post Editor
7. User Profile
8. Settings

图片显示开关默认关闭。

头像可以在 Web UI 中显示，但不要求终端客户端显示图像。

## 6.3 Go TUI 客户端

使用 Bubble Tea。

主要交互：

- 上下键浏览列表。
- Enter 打开主题。
- Esc 返回。
- N/P 翻页。
- R 回复。
- W 创建主题。
- E 编辑自己的帖子。
- Q 退出。

不需要在服务器端实现终端布局。

# 7. 图片与安全

## 7.1 头像

- 支持 JPEG、PNG、WebP。
- 最大 512 KiB。
- 服务端解码和重新编码。
- 使用服务器生成的文件名。
- 不信任上传文件扩展名和 Content-Type。

## 7.2 帖子图片

Markdown：

markdown

```markdown
![Example](https://example.com/image.jpg)
```

默认不加载外部图片。

Web 用户可以自行开启显示。

Telnet 和 TUI 客户端只显示图片说明和链接。

## 7.3 Web 安全规则

| 风险 | 处理 |
| --- | --- |
| XSS | Markdown HTML 禁用，输出安全渲染 |
| JavaScript 注入 | 禁止 |
| HTML 注入 | 禁止原始 HTML |
| 恶意 URL | 限制 URL Scheme |
| CSRF | WebSocket Origin 检查及认证 |
| 暴力登录 | 限制登录频率 |
| 文件路径攻击 | 服务端生成文件路径 |
| 超大请求 | 服务端限制消息和文件大小 |
| 密码泄露 | Argon2id |
| 图片攻击 | 解码、重新编码 |

另外，WebSocket 连接需要限制每个用户的连接数量，并正确处理断开连接和 Session 过期。

# 8. 搜索设计

第一版使用 SQLite FTS5。

帖子正文保存为 Markdown 文件，搜索时将标题和正文纯文本同步到 FTS5 索引。

FTS5 只是可重建的索引，不是正文的权威存储。

搜索范围：

- 主题标题。
- 主题正文。
- 回复正文。

不引入 Elasticsearch。

日文和中文分词质量需要通过实际数据验证，不把复杂分词系统作为第一阶段的前置条件。

# 9. AI Agent 与 MCP

这部分列入未来规划，不进入 v1 的实现范围。

未来 MCP Server 只提供只读工具：

| Tool | 功能 |
| --- | --- |
| `list_boards` | 获取版面 |
| `search_posts` | 搜索讨论 |
| `get_thread` | 获取完整主题及回复 |
| `get_post` | 获取单篇帖子 |
| `get_user_profile` | 获取公开资料 |

Agent 不拥有默认发帖和编辑权限。

MCP Adapter 可以独立运行，通过 stdio 或 Streamable HTTP 提供 MCP 服务。

它直接调用 BBS 的数据和业务逻辑，不需要额外增加 REST API。

特别需要注意：Agent 应当只能读取公开内容，并保留作者、时间和原始帖子链接，不能把隐藏内容或私人草稿暴露给 Agent。

# 10. 配置文件

建议使用 TOML。

初始配置：

toml

```toml
[server]
http_addr = ":8080"

[storage]
data_dir = "./data"

[posts]
max_size = 65536
allow_edit = true
allow_delete = false

[drafts]
max_size = 65536

[images]
show_by_default = false
allow_external_images = true
max_avatar_size = 524288

[security]
max_connections_per_user = 3

[search]
enabled = true
```

配置文件缺失时使用合理默认值。

修改配置后重启生效。

# 11. 项目目录

遵循 Small Software 原则，第一版尽量减少文件数量。

text

```text
retro-bbs/
│
├── main.go
├── go.mod
├── config.toml
│
├── web/
│   ├── index.html
│   ├── style.css
│   ├── app.js
│   └── vendor/
│       └── xterm/
│
└── data/
    ├── bbs.db
    ├── posts/
    ├── drafts/
    ├── avatars/
    └── tmp/
```

Go 代码初期可以全部放在 `main.go`。

只有当文件长度和职责确实影响可读性时才拆分。

不提前创建：

- services/
- repositories/
- factories/
- interfaces/
- adapters/
- domain/
- infrastructure/

未来确实需要 MCP 时再增加对应程序。

# 12. 开发计划

采用逐阶段交付，每一阶段都必须可以运行。

## Development Roadmap

1. Phase 1 — Minimal Server
   第一目标- Go HTTP Server
   - 静态 Web 页面
   - WebSocket 连接
   - JSON 请求/响应
   - 简单 Welcome Menu
   验收：浏览器连接服务器，可以看到 BBS 欢迎画面。
2. Phase 2 — Database & Accounts
   - SQLite 初始化
   - 用户注册
   - 登录和登出
   - 密码哈希
   - Session 管理
   验收：用户可以注册、登录和重新连接。
3. Phase 3 — BBS Core
   - 创建版面
   - 主题列表
   - 阅读主题
   - 创建主题
   - 回复
   - 编辑自己的帖子
   验收：完成基本 BBS 交流流程。
4. Phase 4 — File Storage
   - Markdown 保存
   - 草稿
   - 帖子大小配置
   - 文件与数据库一致性检查
   验收：重启服务器后帖子仍然完整。
5. Phase 5 — Web UI
   - xterm.js
   - 颜色
   - 菜单和分页
   - Markdown 阅读
   - 图片开关
   - Profile 和头像
   验收：完整浏览器 BBS。
6. Phase 6 — Native TUI
   - Go CLI
   - Bubble Tea
   - WebSocket
   - 键盘操作
   - 文章阅读与编辑
   验收：不打开浏览器也能完整使用 BBS。
7. Phase 7 — Search & Hardening
   - FTS5
   - XSS 检查
   - 权限检查
   - 连接限制
   - 备份和恢复测试
   验收：核心功能安全且数据可恢复。
8. Future — MCP
   - MCP Server
   - Agent 搜索
   - Agent 读取主题
   - 来源引用
   验收：AI Agent 能读取公开讨论。

# 13. 测试与验收标准

第一版完成前至少验证以下情况：

| 测试 | 预期结果 |
| --- | --- |
| 注册新用户 | 成功 |
| 重复用户名注册 | 拒绝 |
| 错误密码登录 | 拒绝 |
| 未登录发帖 | 拒绝 |
| 编辑别人的帖子 | 拒绝 |
| 创建超长帖子 | 拒绝 |
| 保存草稿 | 成功 |
| 重启服务器 | 数据保留 |
| 修改帖子 | 更新成功 |
| 隐藏帖子 | 普通用户不可见 |
| 恶意 HTML | 不执行 |
| 恶意 Markdown URL | 不允许 |
| WebSocket 非法 Origin | 拒绝 |
| 搜索帖子 | 返回匹配结果 |

另外，应该测试 SQLite 与文件系统在服务器异常退出时的恢复情况。

# 14. 最终冻结的设计决策

Retro BBS v1.0

Design Baseline

| 决策 | 结果 |
| --- | --- |
| 后端 | Go |
| Web | xterm.js |
| CLI | Bubble Tea |
| 通信 | WebSocket + JSON |
| 数据库 | SQLite |
| 正文 | Markdown 文件 |
| 默认帖子大小 | 64 KiB，可配置 |
| 颜色 | 有限 ANSI SGR |
| 图片 | 可选显示 |
| 删除 | 普通用户禁止 |
| 搜索 | SQLite FTS5 |
| REST | 不实现 |
| Telnet | 暂不实现 |
| MCP | 后续独立增加 |

## 15. 实施时必须遵守的 Small Software 检查清单

每次增加功能之前，先问：

- [ ]这是当前明确需要的功能吗？
- [ ]Go 标准库是否已经能解决？
- [ ]是否真的需要第三方依赖？
- [ ]是否可以用更少的文件实现？
- [ ]是否引入了没有实际用途的 Interface？
- [ ]是否为了未来假设增加了抽象层？
- [ ]是否有可以删除的重复代码？
- [ ]是否有实际运行和测试？

最终目标不是做出一个架构漂亮的系统，而是做出一个十年后你仍然愿意维护的 BBS。

## 设计阶段完成

下一步建议先不要生成所有代码，而是进入 Phase 1：

实现一个真正能运行的 Go WebSocket Server，以及一个 xterm.js Web 客户端。

第一阶段只需要三个成果：

1. `go run main.go` 可以启动服务器。
2. 浏览器可以连接 WebSocket。
3. 屏幕显示 RETRO BBS 欢迎菜单，并可以通过键盘输入命令。

等这个最小版本运行成功，再进入数据库和用户系统。

这样最符合你制定的开发规范。