# task141-firehydraulics — Benzhi 评测说明

## 业务问题

本服务是一个**消防喷淋系统水力计算与合规引擎**。面向消防工程设计院与消防施工单位，在一个后端计算引擎里维护消防工程项目、喷淋/消火栓系统、水力管网（节点与管段）、水源（市政/水池/水箱）、消防水泵与危险等级，按 NFPA 13 树形水力计算法计算各节点流量与压力，校验最不利点喷头是否满足设计密度与最小工作压力，比对供水曲线与需求曲线，并管理从设计→送审→安装→水压试验→验收→在役→故障降级→恢复的完整状态机与检查/试验记录。进程重启后能从数据库权威输入重放出一致的水力计算结果与合规判定。

主要输入：项目（含危险等级）、系统（喷淋/消火栓 + 基准点高程 + 设计密度 + 作用面积）、节点（喷头 k 系数 / 水源 / 三通 / 排水）、管段（直径/长度/C/当量）、水源曲线（流量→残压点）、消防水泵（额定/堵转/150%）。

主要输出：各节点压力与流量（节点表）、基准点总流量与所需压力、供水可用压力与富余、NFPA 13 合规校验逐条结果、系统生命周期状态与事件历史、水压试验/验收/检查记录、故障降级与补偿措施。

## 标准本地命令

```bash
go build ./...        # 编译
go run . --addr=:8080 --db=firehydraulics.db   # 启动服务
go run . --migrate-only                          # 仅建表
go test ./...        # 测试
go run . --smoke-test  # 自检(请求页面+业务API,自行退出)
```

启动后前端页面：`http://localhost:8080/`（覆盖：建项目→建系统→跑水力计算→NFPA合规→看综合报告 的真实读写流程）。

## Docker 构建

构建脚本接收两个参数：镜像名、目标架构。

```bash
# linux/amd64
bash ./build_benzhi_docker.sh go-task-benzhi:amd64 linux/amd64
docker run --rm go-task-benzhi:amd64 go version
docker run --rm go-task-benzhi:amd64 --smoke-test

# linux/arm64
bash ./build_benzhi_docker.sh go-task-benzhi:arm64 linux/arm64
docker run --rm go-task-benzhi:arm64 go version
docker run --rm go-task-benzhi:arm64 --smoke-test
```

进入容器交互：`docker run -it go-task-benzhi:amd64`

前端经 `//go:embed web` 一并打入二进制（原生 HTML/CSS/JS，无 Node 构建步骤）。镜像内验证页面与业务 API：

```bash
docker run --rm go-task-benzhi:amd64 --smoke-test
```

> `--smoke-test` 实际请求前端页面 `/` 与页面使用的业务 API（建项目→建系统→建管网→建水源→计算→合规→送审→批准→水压试验→验收→在役→故障降级→恢复→重启重放），执行后自行退出。
