# task-queue

把慢活从 HTTP 请求里挪到后台去干的任务队列系统。

---

## 目录说明

| 路径 | 作用 | 状态 |
|---|---|---|
| `cmd/api/` | HTTP 入口，Gin | 已铺好 |
| `internal/handler/` | 接口处理，现在返回假数据 | 已铺好，M2 改成真数据 |
| `internal/model/` | 任务表结构 | 已铺好 |
| `internal/store/` | MySQL 连接 | **空的，你要写** |
| `internal/queue/` | Redis 队列 | 第 2 周再建 |
| `cmd/worker/` | 后台干活的进程 | 第 2 周再建 |
| `Dockerfile` | 镜像构建 | v1 能跑但很糟，**第 4 天改造它** |
| `docker-compose.yml` | 服务编排 | 现在只有 MySQL，**M3 你要加 api** |

---

## 第一步：装依赖

```
go mod tidy
```

慢了就先设代理：

```
go env -w GOPROXY=https://goproxy.cn,direct
```

---

## M1　空骨架 + Docker

- [ ] `go run ./cmd/api` 能起来
- [ ] `curl localhost:8080/health` 返回 `{"ok":true}`
- [ ] **手写一遍 Dockerfile**（不许看仓库里那份）
- [ ] `docker build` 成功，`docker run -p 8080:8080` 后 curl 能通
- [ ] `docker exec -it <id> sh` 进去，看进程、看文件、看环境变量
- [ ] 改一行代码重新构建，解释第二次为什么快

**验收：能逐行说清 Dockerfile 每一行的作用。**

---

## M2　接上 MySQL

- [ ] 用 `docker run` 起一个 mysql:8（命令行，先不写 compose）
- [ ] 实现 `internal/store/Init()`
- [ ] 用 `AutoMigrate` 把 `model.Task` 建成表
- [ ] `POST /tasks` 真的写进表，`GET /tasks/:id` 真的查出来
- [ ] 记录不存在时怎么返回？想清楚再写
- [ ] **故意把密码写错一次**，看报什么错

**验收：能用客户端查到 GORM 写进去的记录。**

---

## M3　compose 编排

- [ ] 在 `docker-compose.yml` 里加上 api 服务
- [ ] 让 api 连上 MySQL —— 注意 `.env.example` 里那个地址进了容器还对不对
- [ ] `docker compose up -d` 一条命令起全部
- [ ] `docker compose down` 再 `up`，数据还在
- [ ] 能解释容器之间靠什么互相找到

---

## 这一周不做

Redis、worker、K8s、前端。忍住。

---

## 三条铁律

1. 代码自己写，卡住贴报错。
2. 每个「为什么」答不上来就停下查，不许往前走。
3. Dockerfile 和 compose 手写，不许搜现成抄。
