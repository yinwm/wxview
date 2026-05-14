# Weview

Weview 是一个把本机微信数据变成可访问数据源的工具。

这个项目的目标不是做另一个微信客户端，而是把属于自己的微信数据整理成可以被程序读取、检索和管理的数据源，用来管理自己的数字资产。这里的数字资产包括联系人、群、消息、媒体、文件、链接、关系网络，以及后续可以从这些数据中提取出来的知识、记录和上下文。

V1 先从 macOS 微信 4.x 的联系人数据开始：自动获取 `contact/contact.db` 的数据库 key，解密联系人数据库，维护本地缓存，并通过 CLI 输出联系人和群。

## 设计目标

- 本地优先：微信数据、key、解密缓存都保存在本机。
- 面向自动化：CLI 的 JSON、JSONL、CSV 输出方便其他 AI、脚本或系统调用。
- 数据层复用：CLI 只是一个 controller。后续如果增加 Web API，也应该复用同一套 service 和缓存数据。
- 渐进实现：V1 只做联系人闭环，先把 key、解密、缓存、查询链路跑通。
- 明确边界：V1 不做 WAL patch，不做公开 Web API，不读取消息和媒体。

## 当前能力

- 自动检测 macOS 微信账号目录：
  `~/Library/Containers/com.tencent.xinWeChat/Data/Documents/xwechat_files/<account>/db_storage`
- 自动获取并验证 `contact/contact.db` 的 SQLCipher raw key。
- 解密主数据库 `contact/contact.db` 到本地缓存：
  `~/.weview/cache/<account>/contact/contact.db`
- 运行本地 daemon，前台常驻维护联系人缓存；`~/.weview/weview.sock`
  只作为内部控制通道。
- 通过 CLI 查询联系人、普通私聊联系人、群和其他联系人表记录。
- 支持 `table`、`json`、`jsonl`、`csv` 输出。
- 支持搜索、精确 username 查询、分页、排序和计数。

## 支持命令

### `weview init`

首次使用时运行，一般只需要执行一次。

```sh
sudo go run ./cmd/weview init
```

它会检测微信账号目录，读取联系人数据库 salt，扫描正在运行的微信进程来获取数据库 key，验证成功后保存到：

```sh
~/.weview/keys.json
```

### `weview daemon`

管理本地联系人缓存维护进程。

```sh
go run ./cmd/weview daemon
```

当前 daemon 只支持这 4 种形式：

```sh
go run ./cmd/weview daemon          # 显示 daemon 帮助
go run ./cmd/weview daemon start    # 后台启动 daemon
go run ./cmd/weview daemon stop     # 停止后台 daemon
go run ./cmd/weview daemon status   # 检查 daemon 是否正在响应 health
```

`weview daemon` 和 `weview daemon --help` 行为一样，只显示帮助，不启动进程。

`weview daemon start` 会后台启动 daemon。如果 daemon 已经在运行，会给出
`status: already running`，不会重复启动。后台 daemon 会先确保 key 可用，把
`contact/contact.db` 解密到本地缓存，然后监听主数据库文件变化；发现变化后，
会重新刷新缓存。daemon 日志写入 `~/.weview/weview.log`。

`weview daemon stop` 会停止后台 daemon；如果 daemon 没有运行，会输出
`status: stopped`。

daemon 当前没有其他 flags；除了 `-h` / `--help` / `help` 外，多余参数都会报错。

V1 不处理 `.db-wal`，所以这是准实时刷新：微信把变化写回主数据库后，缓存才会刷新。

daemon 使用本地 Unix socket：

```sh
~/.weview/weview.sock
```

这个 socket 是内部控制通道，不是公开 Web API，也不是联系人查询接口。daemon
只负责维护缓存、响应 `health` 和 `refresh_contacts` 这类内部请求，不负责
`contacts` 查询。

`weview contacts ...` 始终直接读取本地解密缓存；daemon 是否运行只影响缓存是否
会被常驻进程自动维护。

### `weview contacts`

查询联系人。

```sh
go run ./cmd/weview contacts --format table
go run ./cmd/weview contacts --format json
go run ./cmd/weview contacts --format jsonl
go run ./cmd/weview contacts --format csv
```

`weview contacts` 不带参数时只显示帮助，不直接查询数据。要查询数据，需要显式传入 `--format`、`--kind`、`--count` 等参数。

`weview contacts` 始终直接读取本地解密缓存。使用 `--refresh` 时，如果 daemon 正在运行，会先请求 daemon 刷新缓存；如果 daemon 没运行，就在当前进程里刷新一次缓存。

常用查询：

```sh
go run ./cmd/weview contacts --kind friend --format table
go run ./cmd/weview contacts --kind chatroom --format table
go run ./cmd/weview contacts --kind friend --query AI --limit 20 --format json
go run ./cmd/weview contacts --username wxid_xxx --format json
go run ./cmd/weview contacts --kind friend --count
go run ./cmd/weview contacts --refresh --format json
```

面向 AI 或其他系统调用时，优先使用：

```sh
go run ./cmd/weview contacts --kind friend --limit 100 --offset 0 --sort username --format json
go run ./cmd/weview contacts --kind chatroom --format jsonl
go run ./cmd/weview contacts --kind friend --format csv
```

完整参数说明请运行：

```sh
go run ./cmd/weview contacts --help
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

## 本地状态

- 配置目录：`~/.weview/`
- key 文件：`~/.weview/keys.json`
- 联系人缓存：`~/.weview/cache/<account>/contact/contact.db`
- daemon socket：`~/.weview/weview.sock`

CLI 不会打印完整数据库 key，只会打印 fingerprint。

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

## V1 边界

当前版本只支持：

- macOS 微信 4.x
- `contact/contact.db`
- 联系人和群查询

当前版本不支持：

- Windows / Linux
- macOS 微信 3.x
- 消息查询
- 图片、语音、文件、视频等媒体解码
- WAL patch
- 公开 Web API

这些能力后续可以在同一套 key、decrypt、cache、service 架构上继续扩展。
