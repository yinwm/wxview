# Weview

Weview 是一个把本机微信数据变成可访问数据源的工具。

这个项目的目标不是做另一个微信客户端，而是把属于自己的微信数据整理成可以被程序读取、检索和管理的数据源，用来管理自己的数字资产。这里的数字资产包括联系人、群、消息、媒体、文件、链接、关系网络，以及后续可以从这些数据中提取出来的知识、记录和上下文。

V1 先从 macOS 微信 4.x 的联系人数据开始：自动获取支持库的数据库 key，解密联系人和消息相关数据库，维护本地缓存，并通过 CLI 输出联系人、群和按明确 username 查询的聊天记录。

## 设计目标

- 本地优先：微信数据、key、解密缓存都保存在本机。
- 面向自动化：CLI 的 JSON、JSONL、CSV 输出方便其他 AI、脚本或系统调用。
- 数据层复用：CLI 只是一个 controller。后续如果增加 Web API，也应该复用同一套 service 和缓存数据。
- 渐进实现：V1 先做联系人闭环，再扩展明确 username 的消息读取和消息相关缓存维护。
- 明确边界：V1 不做 WAL patch，不做公开 Web API；消息能力先做记录读取、常见 XML 摘要和图片/视频本地解析，语音、文件等媒体文件本体继续后续扩展。

## 当前能力

- 自动检测 macOS 微信账号目录：
  `~/Library/Containers/com.tencent.xinWeChat/Data/Documents/xwechat_files/<account>/db_storage`
- 自动获取并验证当前支持数据库的 SQLCipher raw key：必需库包括
  `contact/contact.db` 和 `message/message_[number].db`；辅助库
  `message/message_fts.db`、`message/message_resource.db`、
  `message/message_revoke.db` 按 best-effort 准备，失败时只输出 warning。
- 解密主数据库 `contact/contact.db` 到本地缓存：
  `~/.weview/cache/<account>/contact/contact.db`
- 解密消息相关数据库到本地缓存：
  `~/.weview/cache/<account>/message/`
- 运行本地 daemon，后台维护联系人和消息缓存；`~/.weview/weview.sock`
  只作为内部控制通道。
- 通过 CLI 查询联系人、普通私聊联系人、群和其他联系人表记录。
- 通过 CLI 按 username 查询 `message/message_*.db` 的聊天记录，支持时间范围、分页和机器可读输出。
- 通过 CLI 按时间范围查询一批会话，并合并成跨会话时间线。
- 支持 `table`、`json`、`jsonl`、`csv` 输出。
- 支持搜索、精确 username 查询、分页、排序和计数。
- `messages` 和 `timeline` 的 `json` 输出使用 `{meta, items}` envelope；
  `meta.schema_version` 标识 envelope 契约版本，`meta.timezone` 标识本地时间解释；
  `jsonl`、`csv`、`table` 仍然只输出记录行。

## 本地构建

```sh
task build
```

构建产物在：

```sh
./bin/weview
```

常用检查：

```sh
task check   # go test ./... + go vet ./...
task clean   # 删除 bin/
```

## 支持命令

### `weview init`

首次使用时运行，一般只需要执行一次。

```sh
sudo ./bin/weview init
```

它会检测微信账号目录，枚举必需数据库和辅助数据库，读取每个数据库 page 1 salt，扫描正在运行的微信进程来获取缺失的数据库 key，验证成功后保存到：

```sh
~/.weview/cache/<account>/keys.json
```

`keys.json` 绑定账号保存，使用 `version: 1`，内容是该账号的 `account`、`data_dir` 和按 DB 相对路径索引的 key。

`init` 默认只输出账号、路径、key 数量和 warning 摘要。需要逐库查看 fingerprint 和 reused/scanned 状态时，再加 `--verbose`。

如果某个辅助库（例如 `message/message_revoke.db`）当前没有在微信进程内找到 key，`init` 会输出 warning 并继续保留其他已准备好的 key。联系人和数字消息分片 key 仍然是必需项，缺失时需要重新运行 `sudo weview init`。

### `weview daemon`

管理本地联系人缓存维护进程。

```sh
./bin/weview daemon
```

当前 daemon 只支持这 4 种形式：

```sh
./bin/weview daemon          # 显示 daemon 帮助
./bin/weview daemon start    # 后台启动 daemon
./bin/weview daemon stop     # 停止后台 daemon
./bin/weview daemon status   # 检查 daemon 是否正在响应 health
```

`weview daemon` 和 `weview daemon --help` 行为一样，只显示帮助，不启动进程。

`weview daemon start` 会后台启动 daemon。如果 daemon 已经在运行，会给出
`status: already running`，不会重复启动。后台 daemon 会先确保 key 可用，把
当前 WeChat 进程正在打开的账号对应的 `contact/contact.db` 和消息相关数据库解密到本地缓存，然后监听主数据库文件变化；发现变化后，会重新刷新缓存。daemon 运行中如果微信切换账号，会重新解析当前 WeChat 打开的 `db_storage` 路径并切到对应账号目录。刷新时会读取
`~/.weview/cache/<account>/mtime.json`，源 DB 的 size、mtime 和 salt 没变且 cache 已存在时会跳过解密。daemon 日志写入 `~/.weview/weview.log`。

`weview daemon stop` 会停止后台 daemon；如果 daemon 没有运行，会输出
`status: stopped`。

daemon 当前没有其他 flags；除了 `-h` / `--help` / `help` 外，多余参数都会报错。

V1 不处理 `.db-wal`，所以这是准实时刷新：微信把变化写回主数据库后，缓存才会刷新。

daemon 使用本地 Unix socket：

```sh
~/.weview/weview.sock
```

这个 socket 是内部控制通道，不是公开 Web API，也不是联系人查询接口。daemon
只负责维护缓存、响应 `health`、`refresh_contacts`、`refresh_messages` 这类内部请求，不负责 `contacts` 或 `messages` 查询。

`weview contacts ...` 始终直接读取本地解密缓存；daemon 是否运行只影响缓存是否
会被常驻进程自动维护。

### `weview contacts`

查询联系人。

```sh
./bin/weview contacts --format table
./bin/weview contacts --format json
./bin/weview contacts --format jsonl
./bin/weview contacts --format csv
```

`weview contacts` 不带参数时只显示帮助，不直接查询数据。要查询数据，需要显式传入 `--format`、`--kind`、`--count` 等参数。

`weview contacts` 始终直接读取本地解密缓存。使用 `--refresh` 时，如果 daemon 正在运行，会先请求 daemon 刷新缓存；如果 daemon 没运行，就在当前进程里刷新一次缓存。

常用查询：

```sh
./bin/weview contacts --kind friend --format table
./bin/weview contacts --kind chatroom --format table
./bin/weview contacts --kind friend --query AI --limit 20 --format json
./bin/weview contacts --username wxid_xxx --format json
./bin/weview contacts --kind friend --count
./bin/weview contacts --refresh --format json
```

面向 AI 或其他系统调用时，优先使用：

```sh
./bin/weview contacts --kind friend --limit 100 --offset 0 --sort username --format json
./bin/weview contacts --kind chatroom --format jsonl
./bin/weview contacts --kind friend --format csv
```

完整参数说明请运行：

```sh
./bin/weview contacts --help
```

#### 输出字段

- `username`：微信内部 username，例如 `wxid_*` 或 `*@chatroom`。
- `alias`：微信号或别名。
- `remark`：你给联系人设置的备注。
- `nick_name`：联系人昵称。
- `head_url`：头像图片 URL。
- `kind`：`friend`、`chatroom` 或 `other`。

`kind` 的含义：

- `friend`：普通私聊联系人。
- `chatroom`：群。
- `other`：公众号、企业联系人、非好友群成员、系统联系人等其他联系人表记录。
- `all`：查询参数，表示不过滤。

### `weview messages`

按明确 username 查询聊天记录。

```sh
./bin/weview messages --username wxid_xxx --format table
./bin/weview messages --username wxid_xxx --date today --limit 100 --format json
./bin/weview messages --username wxid_xxx --start "2026-05-01" --end "2026-05-14" --format json
./bin/weview messages --username wxid_xxx --after-seq 1773421286000 --limit 100 --format jsonl
./bin/weview messages --username 123@chatroom --limit 100 --offset 0 --format jsonl
./bin/weview messages --username wxid_xxx --source --refresh --format json
```

`weview messages` 不带参数时只显示帮助。查询时必须显式传入 `--username`。
`--username` 始终按普通会话目标处理；即使这个值等于当前登录账号 username，也不会被特殊拦截。

时间范围参数：

- `--date today|yesterday|YYYY-MM-DD`：选择一个本地自然日，不能和
  `--start` / `--end` 同时使用。
- `--start`：包含起点。
- `--end`：包含终点；日期格式如 `2026-05-14` 会包含当天 23:59:59。
- 支持 Unix 秒、`YYYY-MM-DD`、`YYYY-MM-DD HH:MM`、`YYYY-MM-DD HH:MM:SS` 和 RFC3339。

分页参数：

- `--after-seq`：只返回 `seq` 大于该值的消息，用来做“从上一条之后继续读”的游标。
- `--limit`：按全局时间正序合并所有分片后最多返回多少条；`0` 表示不限制。
- `--offset`：按全局时间正序合并所有分片后跳过多少条。

机器调用建议使用 `--format json`，并在有下一页时直接执行
`meta.next_args`。`--offset` 适合人工临时查看；消息持续变化或做可靠分页时，
优先使用 `next_after_seq` / `next_args`。

输出字段：

- 默认输出聊天历史需要的字段：`id`、`chat_username`、`from_username`、`direction`、`is_self`、`is_chatroom`、`seq`、`server_id`、`create_time`、`time`、`type`、`sub_type`、`content`、`content_detail`、`content_encoding`。
- `messages` 和 `timeline` 的消息行字段一致，并额外包含会话元信息：
  `chat_kind`、`chat_display_name`、`chat_alias`、`chat_remark`、
  `chat_nick_name`。`chat_display_name` 按 `remark > nick_name > alias >
  username` 生成；联系人缓存里找不到时回退为 `chat_username`，
  `chat_kind` 为 `unknown`。
- `chat_username` 是会话用户名，也就是 `--username` 查询对象；`from_username` 是实际发消息的人。群消息会从正文前缀里解析真实发送者。
- `direction` 是 `out`、`in` 或 `unknown`；`is_self` 表示这条消息是否来自本机账号。
- `content` 是 zstd 解码后的原始正文，是保留给程序使用的原始内容；XML 仍然是 XML，群消息前缀也会保留，不会被改成摘要。
- `content_detail` 是解析后的便利字段，用来让人和程序更容易消费常见消息类型；它不能替代 `content`。
- 对普通文本，`content_detail` 通常包含 `type: "text"` 和 `text`。
- 对链接、文件、小程序、视频号等 appmsg XML，`content_detail` 会解析 `type`、`text`、`title`、`desc`、`url`、`source_username`、`source_display_name` 等字段。
- 对图片消息，`content_detail` 会解析 `type: "image"`、`text: "[图片]"`、`md5`、`origin_md5`、`length`、`cdn_thumb_url`、`cdn_thumb_length`、`cdn_thumb_width`、`cdn_thumb_height`、`cdn_mid_img_url`、`cdn_mid_width`、`cdn_mid_height`、`cdn_hd_width`、`cdn_hd_height`、`hevc_mid_size` 等存在且非零的 XML 元数据。
- 对视频消息，`content_detail` 会解析 `type: "video"`、`text: "[视频]"`、`md5`、`new_md5`、`raw_md5`、`origin_source_md5`、`length`、`raw_length`、`play_length`、`cdn_video_url`、`cdn_raw_video_url`、`cdn_thumb_url`、`cdn_thumb_length`、`cdn_thumb_width`、`cdn_thumb_height` 等存在且非零的 XML 元数据；CDN AES key 不会复制到便利字段里。
- 图片和视频消息会自动解析本地媒体路径；普通图片/视频直接返回本地路径，微信 `.dat` 图片会解密到 `~/.weview/cache/<account>/media/`，可识别的 `.dat` 视频也会规范化到同一目录。解析结果也放在 `content_detail`：`media_status`、`path`、`source_path`、`decoded`、`thumbnail`、`thumbnail_path`、`thumbnail_source_path`、`thumbnail_decoded`、`width`、`height`、`media_reason`。`text` 只保留语义摘要 `[图片]` 或 `[视频]`；`media_status=resolved` 且 `path` 存在时，`path` 才是可以直接打开的本地媒体路径。
- `--source` 才输出 DB/table/local row 等来源字段，正常看聊天记录不需要。

`--format json` 返回 envelope：

```json
{
  "meta": {
    "schema_version": 1,
    "timezone": "Asia/Shanghai",
    "mode": "messages",
    "username": "wxid_xxx",
    "limit": 100,
    "returned": 100,
    "has_more": true,
    "next_after_seq": 1773421988000,
    "next_args": ["messages", "--username", "wxid_xxx", "--after-seq", "1773421988000", "--limit", "100", "--format", "json"]
  },
  "items": []
}
```

运行行为：

- 命令读取本地解密缓存里的 `message/message_*.db`。
- 同一个 username 的消息可能散落在多个 `message_N.db` 分片中，命令会先合并再排序和分页。
- 默认排序是 `create_time` 正序，再按 `seq`、本地行 id、分片名稳定排序。
- V1 不维护消息索引；较大的单聊/群聊和较宽时间范围会更慢。机器分页优先使用
  `--after-seq` 或 `meta.next_args`，避免用大 `--offset` 反复扫描。
- 默认查询只读取现有完整缓存；自动刷新由 daemon 负责维护。
- `--refresh` 会显式刷新消息正文分片和消息辅助库，优先请求 daemon；daemon 不在时在当前进程刷新。
- 如果缺少对应 message DB 的 key，命令会提示先运行 `sudo weview init`，不会在查询路径临时扫描 key。
- 当前支持图片和视频的本地路径解析。图片 `.dat` 会自动解密；视频会先查 `message_resource.db` 的 hardlink 元数据，再回退到 WeChat 本地目录 best-effort 查找，普通视频直接返回路径，可识别的 `.dat` 视频会规范化到媒体缓存。语音、文件等媒体文件本体需要后续继续接入 resolver。

### `weview timeline`

按时间范围查询一批会话，并合并成一条全局时间线。这个命令适合给 AI
或脚本读取“今天所有 AI 群聊内容”“昨天某类客户群发生了什么”这类跨会话上下文。

性能注意：V1 不维护消息索引。`timeline` 会按会话 fan-out 查询本地
`message/message_*.db` 分片，合并后再应用全局 `--limit`，所以宽时间范围或
过宽的会话选择会非常慢。优先使用小时间窗口和明确筛选，例如
`--username wxid_xxx --date today` 或 `--kind chatroom --query AI --date today`；
避免对 `--kind all` 使用宽时间范围。

```sh
./bin/weview timeline --kind chatroom --query AI --date today --limit 200 --format json
./bin/weview timeline --kind chatroom --query AI --start "2026-05-14" --end "2026-05-14" --format json
./bin/weview timeline --kind chatroom --query AI --start "2026-05-14 00:00:00" --end "2026-05-14 23:59:59" --limit 200 --cursor TOKEN --format json
./bin/weview timeline --username wxid_xxx --date yesterday --format jsonl
```

会话选择参数：

- `--kind all|friend|chatroom|other`：按联系人类型选择会话。
- `--query TEXT`：筛选会话的 `username`、`alias`、`remark`、`nick_name`；
  不搜索消息正文。
- `--username TEXT`：精确指定一个会话，不能和 `--kind` / `--query` 同时使用。

时间范围参数：

- `--date today|yesterday|YYYY-MM-DD`：选择一个本地自然日。
- 或同时使用 `--start` 和 `--end`；`--date` 不能与它们混用。

分页参数：

- `--limit`：默认 `200`，最大 `1000`。
- `--cursor`：使用上一次 `meta.next_cursor` 继续读取。继续分页时保留相同查询条件，只追加 `--cursor`。

机器调用建议直接执行 `meta.next_args`，不需要理解 cursor 内部结构。
`matched_chats` 表示筛选命中的会话数量，不表示本页或本时间范围内实际有消息的会话数量。

`timeline --format json` 返回 envelope：

```json
{
  "meta": {
    "schema_version": 1,
    "timezone": "Asia/Shanghai",
    "mode": "timeline",
    "kind": "chatroom",
    "query": "AI",
    "start": "2026-05-14 00:00:00",
    "end": "2026-05-14 23:59:59",
    "limit": 200,
    "returned": 200,
    "matched_chats": 12,
    "has_more": true,
    "next_cursor": "TOKEN",
    "next_args": ["timeline", "--kind", "chatroom", "--query", "AI", "--start", "2026-05-14 00:00:00", "--end", "2026-05-14 23:59:59", "--limit", "200", "--cursor", "TOKEN", "--format", "json"]
  },
  "items": []
}
```

可靠分页建议使用 `--format json` 并读取 `meta.next_args`。`jsonl`、`csv`
和 `table` 只输出 `items`，便于管道处理。

## 未来性能 TODO

下面这些是后续可能的性能优化方向，不属于当前 V1 行为。

- `messages` 查询下推分页：在保持跨 `message_*.db` 分片全局排序正确的前提下，
  按分片使用 `limit + offset` 或游标上界减少读取量，避免大群/长时间范围全量加载后再分页。
- `messages` 优先使用 `sort_seq` / `create_time` 的可用索引：查询最新页、继续页时尽量走
  SQLite 已有索引，减少临时排序和大结果集传输。
- `timeline` 查询预估：增加类似 `--explain` / `--dry-run` 的模式，只返回命中的会话数量、
  时间范围和可能扫描的消息缓存，帮助 AI/脚本在执行昂贵查询前缩小范围。
- `timeline` 分批 fan-out：对多会话查询做更细的批处理和早停策略，优先读取最可能落入当前页的候选行，
  但必须保持 cursor 不重复、不漏数据。
- 可选消息索引库：未来可以增加 `weview index build/status/refresh`，物化一张本地消息表，
  并为 `chat_username + create_time/seq`、`create_time + chat_username` 建普通索引。
  这会同时加速 `messages` 和 `timeline`。
- 可选 FTS5 正文索引：如果新增 `weview search`，可以在同一个索引库中增加 FTS5 表。
  FTS5 主要加速正文搜索；`messages` / `timeline` 的主要加速来自普通时间和会话索引。
- meta 标记快慢路径：有索引后，`messages`、`timeline`、未来 `search` 的 JSON meta 可以加入
  `indexed: true|false`，让调用方知道当前结果来自索引快路径还是 V1 慢路径。

## 本地状态

- 配置目录：`~/.weview/`
- key 文件：`~/.weview/cache/<account>/keys.json`
- 联系人缓存：`~/.weview/cache/<account>/contact/contact.db`
- 消息缓存：`~/.weview/cache/<account>/message/message_*.db`
- 图片解密缓存：`~/.weview/cache/<account>/media/`
- 缓存刷新元数据：`~/.weview/cache/<account>/mtime.json`
- daemon socket：`~/.weview/weview.sock`

CLI 不会打印完整数据库 key。`weview init --verbose` 只打印短 fingerprint。

如果之前用 `sudo` 产生过 root-owned 状态，可以修复一次权限：

```sh
sudo chown -R "$USER":staff ~/.weview
```

## 权限说明

获取 key 需要读取正在运行的微信进程内存。失败时通常需要确认：

- 微信正在运行。
- 使用了 `sudo`。
- macOS 允许当前终端读取目标进程。
- 某些 Hardened Runtime 构建可能需要本地研究环境下重新签名微信并开启 `get-task-allow`。

## 致谢

Weview 的实现过程中参考并受益于这些开源项目：

- 特别感谢 [wechat-decrypt](https://github.com/ylytdeng/wechat-decrypt)：这个项目提供了 macOS 微信数据库解密方向上的关键参考，是 Weview 能够落地本地解密与缓存流程的重要基础。
- 感谢 [wechat-cli](https://github.com/huohuoer/wechat-cli)：它在本地微信数据 CLI 化、查询体验和使用场景上提供了有价值的参考。
- 感谢 [wx-cli](https://github.com/jackwener/wx-cli)：它在命令设计、媒体解析和面向自动化使用的数据输出上提供了有价值的参考。

## V1 边界

当前版本只支持：

- macOS 微信 4.x
- `contact/contact.db`
- 联系人和群查询
- `message/message_*.db`
- 按明确 username 查询聊天记录、常见 XML 摘要和图片/视频本地路径解析

当前版本不支持：

- Windows / Linux
- macOS 微信 3.x
- 语音、文件等媒体文件本体解码
- WAL patch
- 公开 Web API

这些能力后续可以在同一套 key、decrypt、cache、service 架构上继续扩展。
