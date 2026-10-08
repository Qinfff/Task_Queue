# task-queue 复习手册（第 1 周：M1 容器化 + M2 接数据库 + M3 compose 编排）

## 怎么用这份文档

不要从头精读。三步：

1. **扫第〇节 + 第二节** —— 回忆主线（3 分钟）
2. **翻第五节概念卡** —— 找遗忘点（2 分钟）
3. **做第六节自测题** —— 卡住哪题，跳回对应章节

---

## 〇、这一周分别干了什么

| 阶段 | 一句话 | 产出 | 状态 |
|---|---|---|---|
| 准备 | 弄清项目要干什么 | 目标队列系统定义 | 完成 |
| M1 | 把一个 Go 服务装进容器，并让它变小 | `Dockerfile`（镜像 1.9GB → 36MB） | 完成 |
| M2 | 接上 MySQL，接口做真正的增和查 | `store.Init()` + 三个 handler | 完成 |
| M3 | 一条命令起 mysql + api | `docker-compose.yml` | 完成 |

---

## 一、一句话定位

**一个把慢活从 HTTP 请求里挪到后台去干的任务队列系统。**

```text
POST /tasks  →  写一行任务记录到 MySQL  →  返回 201
```

这一行背后实际发生的：Gin 解析请求体 → GORM 生成 INSERT → MySQL 写入 → 返回自增 id。
**第 2 周要补上的是"谁去执行这个任务"** —— 现在只有"登记"，还没有"干活的人"。

---

## 二、一次完整旅程（主线）

### 段 1：从源码到镜像（`docker build`）

| # | 谁 | 干了什么 |
|---|---|---|
| 1 | Docker 客户端 | 把当前目录**打包成"构建上下文"** 发给守护进程 |
| 2 | `.dockerignore` | 决定哪些文件**不发过去**（`.git`、`.env` 被排除） |
| 3 | **builder 阶段** | `FROM golang:1.26 AS builder`，用完整 Go 环境编译 |
| 4 | | `COPY go.mod go.sum ./` → `RUN go mod download`（**顺序决定层缓存**） |
| 5 | | `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o server ./cmd/api` |
| 6 | **运行阶段** | `FROM alpine:latest`，**一个干净的新镜像**，前面全不算数 |
| 7 | | `COPY --from=builder /app/server /app/server` —— **唯一的搬运通道** |

### 段 2：容器启动（`docker compose up -d`）

| # | 谁 | 干了什么 |
|---|---|---|
| 1 | compose | 建**专属网络**（服务之间能用服务名互访） |
| 2 | compose | 建 **volume** `task_queue_mysql-data`，挂到 `/var/lib/mysql` |
| 3 | mysql 容器 | 启动，初始化数据库（20-40 秒） |
| 4 | **healthcheck** | 每 5 秒 `mysqladmin ping`，连续成功 → 标记 `(healthy)` |
| 5 | api 容器 | `depends_on: condition: service_healthy` → **等到 healthy 才启动** |
| 6 | api 容器 | `main()` 读 `MYSQL_DSN` → `store.Init()` → `Ping()` → `AutoMigrate` |

### 段 3：请求进来（`POST /tasks`）

| # | 谁 | 干了什么 |
|---|---|---|
| 1 | Gin 引擎 | 匹配路由 → 穿过 Logger / Recovery 中间件 → `handler.CreateTask` |
| 2 | `ShouldBindJSON` | 解析请求体到 `createTaskRequest`（只有 `type` / `payload`） |
| 3 | `CreateTask` | 组 `model.Task{Status: StatusPending}` |
| 4 | GORM | `store.DB.Create(&task)` → 生成 INSERT |
| 5 | MySQL | 写入，回填自增 id |
| 6 | `CreateTask` | 返回 `201` + `{"task_id":1,"status":"pending"}` |

### 段 4：查询（`GET /tasks/:id`）

| # | 谁 | 干了什么 |
|---|---|---|
| 1 | `GetTask` | `strconv.ParseUint(c.Param("id"), 10, 64)` → 失败则 **400** |
| 2 | GORM | `First(&task, id)` → `WHERE id = ?`（**参数化**） |
| 3 | `GetTask` | `errors.Is(err, gorm.ErrRecordNotFound)` → **404**；其他 → 记日志 + **500** |
| 4 | `GetTask` | 成功 → `200` + 任务对象（**JSON 字段由 `json` tag 决定**） |

### 一句话背下来

```text
docker build(两阶段) → compose up(网络+卷+健康门) → Gin 路由 → 绑定 → GORM → MySQL → 序列化返回
```

### 调用关系总图

```text
docker compose up -d
  |
  +-- [mysql 容器] mysqld ──healthcheck(mysqladmin ping)──> (healthy)
  |
  +-- [api 容器] /app/server
        |
        main()                                     cmd/api/main.go
          +-- godotenv.Load()                      失败仅记日志（容器里没有 .env）
          +-- os.Getenv("MYSQL_DSN")               空则 log.Fatal
          +-- store.Init(dsn)                      internal/store/mysql.go
          |     +-- gorm.Open(mysql.Open(dsn))     懒连接，此处不产生 TCP 连接
          |     +-- db.DB() -> sqlDB               拿到底层 *sql.DB
          |     +-- sqlDB.Ping()                   真正验证连通性
          |     +-- SetMaxOpenConns / ...          连接池四件套
          |     +-- db.AutoMigrate(&model.Task{})  建表/加列（不删列）
          +-- gin.Default()                        内建 Logger + Recovery
          +-- r.Run(addr)
                |
                +-- GET  /health       -> handler.Health
                +-- POST /tasks        -> handler.CreateTask  -> store.DB.Create -> MySQL
                +-- GET  /tasks/:id    -> handler.GetTask     -> store.DB.First  -> MySQL
```

**注意跨进程处**：api 和 mysql 是**两个进程、两条链**，中间只有一次网络往返（容器内网络，地址 `mysql:3306`）。不是一条长链。

---

## 三、分层地图

| 层 | 解决什么问题 | 关键类型 | 文件 |
|---|---|---|---|
| 入口 | 装配和启动，**不含业务逻辑** | `main()` | `cmd/api/main.go` |
| HTTP | 路由、参数解析、状态码 | `gin.Context`、`createTaskRequest` | `internal/handler/task.go` |
| 数据模型 | 表结构 + API 的字段形状 | `Task`、`StatusPending` 等常量 | `internal/model/task.go` |
| 存储 | 连接、连接池、建表 | `store.DB`（`*gorm.DB`） | `internal/store/mysql.go` |
| 构建 | 从源码到镜像 | — | `Dockerfile` |
| 编排 | 多个服务怎么一起跑 | — | `docker-compose.yml` |

**设计哲学：每层只解决自己的问题。**
`handler` 不知道 SQL，`store` 不知道 HTTP，`model` 不知道两者。所以换数据库只动 `store`，换框架只动 `handler`。

---

## 四、各阶段加了什么

| 阶段 | 加了什么 | **不加会出什么事故** | 关键新增 |
|---|---|---|---|
| M1 | 多阶段构建 | 镜像 **1.9GB**，其中 98% 是编译器和源码 —— 每次部署都在传垃圾 | `FROM ... AS builder`、`COPY --from=builder` |
| M1 | 层缓存顺序 | 改一行代码 → **依赖重新下载一遍** | `COPY go.mod go.sum ./` 单独一行 |
| M1 | `CGO_ENABLED=0` | 二进制依赖 glibc，在 alpine 里**报找不到动态库** | 静态编译 |
| M1 | `-trimpath -ldflags="-s -w"` | 二进制多 20-30%，且泄露本地路径 | 去符号表和调试信息 |
| M2 | `Ping()` | **数据库没启动也"连接成功"**，错误推迟到某个业务请求才炸 | 启动即验证 |
| M2 | 连接池四件套 | 无上限连接打满 MySQL；空闲 8 小时后拿到**已被 MySQL 关掉的死连接** | `SetConnMaxLifetime(1h)` |
| M2 | `ParseUint` | 字符串直接进 `First`，**被当成 SQL 条件拼接** | 显式转 `uint64` |
| M2 | `errors.Is` 双分支 | 数据库挂了也报 404，**监控一片绿，排查方向全错** | `ErrRecordNotFound` 单独处理 |
| M3 | 服务名当主机名 | 容器里写 `localhost` 连到**容器自己** | `mysql:3306` |
| M3 | healthcheck + `service_healthy` | `depends_on` 只管启动顺序，api 在 MySQL 就绪前连过去 → **失败退出** | `start_period: 30s` |
| M3 | volume | `docker compose down` 之后**数据全没** | `mysql-data:/var/lib/mysql` |

---

## 五、概念卡（忘了就翻这里）

### 1. 层缓存

- **是什么**：Dockerfile 每条指令是一层，每层有缓存 key。**任何一层失效，它下面所有层全部重跑**
- **没它会怎样**：`COPY . .` 放在最前面 → 改一个字母，依赖下载和编译全部重来
- **关键点**：`COPY go.mod go.sum ./` 单独一行 → 只有依赖变了才重跑下载
- 注意：build 时用 `go mod download`，**不要用 `go mod tidy`**（tidy 会改 go.mod，破坏可复现）

### 2. 多阶段构建

- **是什么**：一个 Dockerfile 里多个 `FROM`，最后的 `FROM` 才是产物
- **没它会怎样**：镜像里带着完整 Go 工具链，**1.9GB**
- **关键点**：不是"删掉"了 1.5GB，而是**根本没带过来**；只有 `COPY --from=builder` 能搬东西

### 3. `CGO_ENABLED=0`

- **是什么**：关闭 cgo，产出静态链接的二进制
- **没它会怎样**：二进制依赖 **glibc**，而 alpine 用 **musl** → 运行时报找不到动态库
- **关键点**：这是"本地能跑、容器跑不了"的三大原因之一（另两个：工具链版本、依赖版本）

### 4. 容器之间用服务名当主机名

- **是什么**：Docker 内建 DNS，compose 里的**服务名**就是容器的主机名
- **没它会怎样**：写 `localhost` → 解析成**容器自己** → 连不上
- **关键点**：
  - 主机上的程序连容器 → `127.0.0.1:3308`（**主机端口**）
  - 容器里的程序连容器 → `mysql:3306`（**服务名 + 容器内端口**）
  - **这两条是完全不同的两条路**

### 5. 端口映射的方向

- **是什么**：`-p 3308:3306` = **主机端口:容器端口**
- **没它会怎样**：以为容器里的服务监听在 3308，其实容器里永远是 3306
- **关键点**：`EXPOSE 8080` **什么都不做**，它只是文档元数据；真正开端口的是 `-p`

### 6. volume

- **是什么**：容器外的存储，挂到容器内某个路径
- **没它会怎样**：容器删了，里面的数据**跟着一起删**
- **关键点**：
  - 必须挂到**程序真正写数据的路径**（MySQL 是 `/var/lib/mysql`）
  - `docker compose down` 保留卷；**`down -v` 连卷一起删**
  - volume（Docker 管）比 bind mount（你管）更适合数据库，尤其 Windows

### 7. healthcheck 与 `service_healthy`

- **是什么**：让容器有"健康 / 不健康"这个状态
- **没它会怎样**：`depends_on: condition: service_healthy` **永远等不到** → api 永不启动
- **关键点**：
  - `start_period` 是**启动宽限期**，期间的失败不计入 `retries`。启动慢的服务**必须配**
  - 不写 `condition` 的 `depends_on` **只保证启动顺序**，不保证对方就绪

### 8. `gorm.Open` 是懒连接

- **是什么**：它只构造对象，**不建立 TCP 连接**
- **没它会怎样**：数据库没启动、密码错，`gorm.Open` **照样返回 nil error**
- **关键点**：必须 `sqlDB.Ping()` 才能验证"网络通 + 认证过 + 库存在"

### 9. 连接池四件套

| 设置 | 不设的后果 |
|---|---|
| `SetMaxOpenConns(25)` | 默认无上限 → 打满 MySQL 的 `max_connections`（默认 151） |
| `SetMaxIdleConns(10)` | 连接反复创建销毁 |
| `SetConnMaxLifetime(time.Hour)` | **默认永生** → 拿到被 MySQL 单方面关掉的死连接 |
| `SetConnMaxIdleTime(10min)` | 空闲连接长期占着 |

- **"周一早上 502"**：周五的连接空闲一周末，MySQL 在 8 小时（`wait_timeout`）时关掉它们，程序不知道，周一拿出来就用 → `invalid connection`
- **关键点**：`ConnMaxLifetime` 必须 **小于** MySQL 的 `wait_timeout`
- 注意：`MaxOpenConns` 要按实例数算 —— 设 100、将来 K8s 扩 5 个副本就是 500 个连接

### 10. `AutoMigrate` 的边界

- **会做**：建表、加列、加索引
- **不会做**：**绝不删列**（怕丢数据）、复杂类型变更、数据迁移
- **代价**：每次启动都查一遍表结构比对，表多了会拖慢启动
- **关键点**：生产用迁移工具（goose / golang-migrate），`AutoMigrate` 只开发用

### 11. GORM 命名策略

| 你写的 | 变成 |
|---|---|
| 结构体 `Task` | 表名 **`tasks`**（复数 + 蛇形） |
| 字段 `LastError` | 列名 **`last_error`** |
| Go `int` | MySQL **bigint**（64 位平台） |

### 12. `gorm` tag 和 `json` tag 是两件事

```go
Status string `gorm:"size:32;not null;index" json:"status"`
```

- `gorm` tag → **怎么变成表结构**
- `json` tag → **怎么变成 JSON**
- **关键点**：加 `json` tag **不需要改数据库**，不用迁移。不加的话字段名会原样输出成 `Status`（PascalCase）

### 13. GORM 的字符串参数陷阱

- **是什么**：`First(&task, x)` 里，如果 `x` 是**字符串**，GORM 会判断：纯数字 → 当主键；**其他 → 当成 SQL 条件表达式直接拼进去**
- **没它会怎样**：`GET /tasks/1=1` 生成 `WHERE 1=1`，**返回表里第一条记录**
- **关键点**：**永远先 `ParseUint` 转成数字**，这样就 100% 走参数化路径

### 14. `errors.Is` 而不是 `==`

- **是什么**：沿错误链查找
- **没它会怎样**：GORM 包了一层错误，`== gorm.ErrRecordNotFound` 判断失败
- **关键点**：判断"是不是某个特定错误"一律用 `errors.Is`

### 15. 状态码 4xx vs 5xx 的判断法则

> **问自己：这个错误，是"客户端给的东西不对"，还是"我给不出答案"？**

| 场景 | 码 |
|---|---|
| `/tasks/999999`（格式合法，资源不存在） | **404** |
| `/tasks/abc`（格式非法） | **400** |
| 数据库连不上 | **500** |

- **为什么重要**：状态码的首要目的是**让客户端能自动决策** —— `4xx` 客户端不该重试，`5xx` 应该重试
- 把"输入非法"标成 500 = 告诉全世界"这是我的临时故障，快来重试"

---

## 六、自测题

| # | 问题 | 一句话答案 |
|---|---|---|
| 1 | 为什么镜像从 1.9GB 降到 36MB？ | 多阶段构建：编译产物单独搬进一个干净的精简镜像，编译器根本不过去 |
| 2 | `COPY go.mod go.sum ./` 为什么单独一行？ | 让"依赖"和"源码"分成两层，改源码不触发重新下载依赖 |
| 3 | 为什么 build 时用 `go mod download` 而不是 `tidy`？ | tidy 会**修改** go.mod，破坏"构建只读、结果可复现" |
| 4 | `CGO_ENABLED=0` 解决什么问题？ | 静态编译，不依赖 glibc，才能在 alpine（musl）里跑 |
| 5 | 容器里的 api 该用什么地址连 mysql？ | `mysql:3306` —— 服务名 + **容器内端口** |
| 6 | 为什么不是 `127.0.0.1:3308`？ | `3308` 是**主机**的门牌号；容器之间走内部网络，用 3306 |
| 7 | `EXPOSE 8080` 做了什么？ | 什么都没做，只是元数据。真正开端口的是 `docker run -p` |
| 8 | `depends_on` 能保证 MySQL 已经就绪吗？ | 不能。它只管**启动顺序**，就绪要靠 healthcheck |
| 9 | `start_period` 是干什么的？ | 启动宽限期，期间检查失败不计入 `retries`（MySQL 初始化要 20-40 秒） |
| 10 | `gorm.Open` 成功说明连上数据库了吗？ | **不说明**。它是懒连接，必须 `Ping()` 才验证 |
| 11 | 为什么 `ConnMaxLifetime` 要小于 8 小时？ | MySQL 的 `wait_timeout` 默认 8 小时会关掉空闲连接，我们要主动换，别等它关 |
| 12 | `AutoMigrate` 会不会删掉多余的列？ | 不会，这是安全设计。删列要手动或用迁移工具 |
| 13 | `Task` 结构体对应哪张表？ | `tasks`（GORM 默认复数蛇形） |
| 14 | `gorm` tag 和 `json` tag 能互相替代吗？ | 不能。一个管表结构，一个管 JSON 形状 |
| 15 | `GET /tasks/1=1` 改之前为什么会返回数据？ | GORM 把非数字字符串当 **SQL 条件**拼接，生成 `WHERE 1=1` |
| 16 | `GET /tasks/abc` 该返回 400 还是 500？ | 400。这是客户端输入非法，不是服务端故障 |
| 17 | `docker compose up -d` 会重新构建镜像吗？ | **不会**。改代码后要用 `up -d --build` |

---

## 七、文件地图 + 常用命令

| 文件 | 内容 |
|---|---|
| `Dockerfile` | 两阶段构建：`builder`（golang:1.26）→ 运行（alpine），非 root 用户 UID 10001 |
| `docker-compose.yml` | 两个服务：`mysql`（含 healthcheck）+ `api`（`build: .`） |
| `.env` | 本地开发的配置（**已被 `.dockerignore` 排除，进不了镜像**） |
| `.env.example` | 配置模板，进 git |
| `cmd/api/main.go` | 装配：加载配置 → 初始化 store → 注册三路由 → 启动 |
| `internal/handler/task.go` | 三个 handler，含全部状态码分支 |
| `internal/model/task.go` | `Task` 结构体 + 5 个状态常量 |
| `internal/store/mysql.go` | `Init()`：连接 + Ping + 连接池 + AutoMigrate |
| `.dockerignore` | 决定哪些文件不进"构建上下文" |

### 常用命令

```bash
cd "/c/Users/QIN/Desktop/Task_Queue"

# 改完代码（固定用 --build）
docker compose up -d --build

# 只校验配置，不启动
docker compose config

# 看状态（两个都要 Up，mysql 要 (healthy)）
docker compose ps

# 看日志
docker compose logs api --tail 50

# 进数据库
docker exec -it task_queue-mysql-1 mysql -uroot -proot taskqueue -e "show tables;"

# 看连接数（接口变慢时第一件事）
docker exec -it task_queue-mysql-1 mysql -uroot -proot -e "show status like 'Threads_connected';"

# 代码检查
gofmt -l . && go vet ./...
```

### 实测数据快照

**镜像体积（改造前后）**

| | v1（单阶段） | v2（多阶段） |
|---|---|---|
| 镜像 | **1.9 GB** | DISK USAGE **36.1MB** / CONTENT SIZE 9.63MB |
| 缩小 | — | **约 53 倍** |

```text
磁盘上的 36MB = alpine 基础层(~8MB) + ca-certificates/tzdata(~4MB) + 你的二进制(二十几MB)
```

**怎么读它**：1.9GB 里只有那二十几 MB 是"必须上生产"的。剩下的是编译器和依赖源码 —— **构建期的工具，不是运行期的必需品**。

**接口实测**

```text
GET  /health      -> 200  {"ok":true}
POST /tasks       -> 201  {"task_id":1,"status":"pending"}
GET  /tasks/1     -> 200  {"id":1,"type":"export","payload":"","status":"pending",...}
GET  /tasks/abc   -> 400  {"error":"invalid task id"}
GET  /tasks/1=1   -> 400  {"error":"invalid task id"}      <- 修复前返回一条真实数据
```

**怎么读它**：`1=1` 那条是**同一个 URL、修复前后两种结果** —— 这是"参数化查询真的生效了"的唯一硬证据，比说一百句"我用了参数化"都有用。

**compose 启动日志（关键三行）**

```text
Container task_queue-mysql-1 Healthy      <- healthcheck 通过
Container task_queue-api-1 Starting
Container task_queue-api-1 Started        <- 它真的等了 mysql
```

**怎么读它**：这三行就是 `depends_on: condition: service_healthy` 生效的**直接证据**。

**volume 持久化验证**

```text
容器创建时间: 2026-09-27T10:24:52Z（即 down + up 之后新建的）
查询结果:     {"id":1,...,"created_at":1790504499,...}   与重建前完全一致
```

**怎么读它**：容器换了新的，数据还是同一条（时间戳一致）→ **命名卷在起作用**。

---

## 八、真实踩过的坑（面试素材）

### A. `go 1.22` 撞上新依赖

- **现象**：`go: go.mod requires go >= 1.25.0 (running go 1.22.12; GOTOOLCHAIN=local)`
- **根因**：骨架里写 `go 1.22`，但本机 Go 是 1.26，`go mod tidy` 拉到的新依赖要求 >= 1.25，于是 tidy **自动把 go 1.22 改写成 1.25.0**；而 Docker 基础镜像是 `golang:1.22`
- **修法**：镜像版本必须 **>= go.mod 声明的版本**
- **可复用原则**：镜像里的工具链版本，必须覆盖代码声明的版本要求

### B. 容器里下载不了 Go 依赖

- **现象**：`dial tcp 142.250.73.113:443: connect: connection refused`（Google 的 IP）
- **根因**：`go env -w GOPROXY=...` 写的是**本机全局配置**，**不会进镜像**。镜像里是默认的 `proxy.golang.org`
- **修法**：Dockerfile 里 `ENV GOPROXY=https://goproxy.cn,direct`
- **可复用原则**：**镜像是一个干净的房间**。你在本机做的所有配置都不会跟进去，Dockerfile 必须是环境的完整声明

### C. 端口 3306 被占用

- **现象**：`bind: Only one usage of each socket address ... is normally permitted`
- **根因**：本机装了 MySQL（`mysqld.exe` 作为 Windows 服务占着 3306）
- **修法**：容器映射到空闲端口 `-p 3308:3306`
- **可复用原则**：起容器前先确认端口占用。三步排查：`docker ps` → `netstat -ano | grep :端口` → 查 PID

### D. 容器名冲突

- **现象**：`Conflict. The container name "/tq-mysql" is already in use`
- **根因**：**`docker run` 失败时，容器已经被创建了**（只是启动失败），名字被占用
- **修法**：`docker rm -f tq-mysql`
- **可复用原则**：`docker run` 报错不等于什么都没留下，`docker ps -a` 才看得见残留

### E. YAML 语法三连

- **现象**：`docker compose config` 报 `services.api.environment.[0]: unexpected type map[string]interface {}`
- **根因**：YAML 用**缩进和空格**表达结构，对空格极度敏感。踩了三个：
  1. `build .` 少了冒号 → `build: .`
  2. `-"8080:8080"` 少了空格 → `- "8080:8080"`
  3. `environment` 里混用列表语法（`-`）和映射语法（`:`）
- **修法**：`docker compose config` 会带行号指出问题
- **可复用原则**：**写完 config 一遍再 up**。`config` 只解析不启动，把问题拦在启动之前

### F. 所有数据库错误都返回 404

- **现象**：`First` 的 error 分支只写了一种情况
- **根因**：只判了 `err != nil`，没区分 `ErrRecordNotFound` 和其他错误
- **后果**：**数据库挂掉时，用户看到"任务不存在"，监控一片绿（404 不算错误）**，排查方向完全错，而且它不报错、只是安静地撒谎
- **修法**：`errors.Is(err, gorm.ErrRecordNotFound)` → 404；其他 → 记日志 + 500
- **可复用原则**：错误处理要按"**谁的责任**"分支，不能一刀切

### G. 字符串 id 带来的注入面

- **现象**：`GET /tasks/1=1` **返回了一条真实数据**
- **根因**：`c.Param("id")` 是字符串，直接传给 `First(&task, id)`。GORM 对**非数字字符串**的处理是"当作 SQL 条件表达式"拼进查询
- **修法**：`ParseUint` 先转 `uint64`，GORM 就 100% 走参数化
- **可复用原则**：**外部输入永远先转型**。类型转对了，注入面自动关闭

### H. JSON 字段名两个风格

- **现象**：`POST /tasks` 返回 `task_id`，`GET /tasks/:id` 返回 `ID`、`Type`、`LastError`
- **根因**：前者是 handler 里手写的 map，后者直接序列化 `model.Task`，而 `Task` 没有 `json` tag → Go 默认原样输出字段名
- **修法**：给 `model.Task` 加 `json:"xxx"` tag（**不用改数据库**）
- **可复用原则**：**对外契约要统一**。同一个 API 里两种命名风格，集成方要写两套解析

### I. `docker compose up -d` 不重建镜像

- **现象**：改了代码跑 `up -d`，行为没变
- **根因**：`up -d` 只在镜像不存在时构建；镜像已存在就直接用
- **修法**：`docker compose up -d --build`
- **可复用原则**：容器里跑的是**镜像**，不是你的源码目录。改源码 ≠ 改镜像

---

## 九、下一步（第 2 周预告）

1. 加 **Redis**（队列）—— 注意本机 6379 被 RAGFlow 的 valkey 占着，要换端口
2. 加 **worker** 进程（和 api **共用同一个镜像**，不同启动命令）
3. Dockerfile 要改成**编译两个二进制**（`cmd/api` + `cmd/worker`）
4. 引入**真正的异步**：`POST /tasks` 只入队，worker 在后台执行 —— 那时才讨论 `201` 还是 `202`
5. 补**联合索引** `(status, created_at)`（现在只有单列索引）
