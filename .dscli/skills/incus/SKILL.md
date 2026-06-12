---
name: incus
description: Incus 容器生命周期管理 — 测试环境快速搭建、隔离、快照回滚、销毁
keywords: incus, container, vm, instance, lxc, testing, test, 测试, 容器, 虚拟机, 镜像, 测试环境, QA, 测试自动化
author: JUN JIE NAN <nanjunjie@gmail.com>
---

# incus — 测试环境管理

Incus 是测试工程师搭建隔离测试环境的利器。与 Docker 面向应用封装不同，
Incus 面向**整个操作系统**（system container / VM），让你在秒级获得一个
完整的、隔离的、可复现的测试环境。

本 skill 围绕测试场景组织：从选择实例类型 → 创建干净的测试环境 →
执行测试 → 维护管理 → 销毁回收。

## 测试场景速查

| 测试需求 | 推荐类型 | 理由 |
|----------|----------|------|
| 单元测试/命令行工具测试 | System Container | 秒级启动，用完即焚 |
| 集成测试（多服务协作） | System Container | 多个容器通过网络互联 |
| 浏览器自动化 / UI 测试 | Virtual Machine | 需要完整 GUI 栈、独立内核 |
| 兼容性矩阵测试 | System Container | 多种发行版快速切换 |
| 数据库/状态测试 | System Container + Snapshot | 快照回滚，测试间状态隔离 |
| 网络/防火墙规则测试 | System Container | 独立网络命名空间 |
| 性能/资源约束测试 | System Container / VM | `limits.cpu/memory` 精准限制 |
| 恶意软件/安全测试 | Virtual Machine | 独立内核隔离，风险可控 |

## 前置条件

### 0. 安装与初始化

```bash
# 安装 incus（Arch Linux）
sudo pacman -S incus

# 将当前用户加入 incus 组
sudo usermod -a -G incus $USER
# 重新登录使组生效，或使用 newgrp incus

# 初始化（交互式）
sudo incus admin init

# 或最小化初始化（默认网桥 + 默认存储池）
sudo incus admin init --auto
```

查看状态：

```bash
incus admin init --dump   # 查看当前配置
incus list                # 查看现有实例
incus storage list        # 查看存储池
```

> **AI 智能体注意**：执行任何 `incus launch` / `incus exec` 等操作前，
> 确认 incus daemon 在运行（`incus list` 不报错即正常）。
> 若报 "permission denied"，用户需加入 `incus` 组。

### 1. 为你的测试场景选择实例类型

| 类型 | 内核 | 启动速度 | 密度 | 测试场景 |
|------|------|---------|------|----------|
| **System Container**（系统容器） | 共享宿主机内核 | 极快（秒级） | 极高 | CLI 测试、环境变量测试、配置测试、多容器集成测试 |
| **Application Container**（应用容器） | 共享宿主机内核 | 快 | 高 | 从 Docker Hub 等 OCI 仓库拉取的服务做冒烟测试 |
| **Virtual Machine**（虚拟机） | 独立内核 | 较慢（分钟级） | 较低 | 浏览器测试、内核相关测试、安全隔离要求高的测试 |

**测试场景快速判断**：

- 需要一个即时可用的干净 Linux 环境跑测试 → **System Container**
- 测试一个 OCI/Docker 镜像的启动和行为 → **Application Container**
- 需要测试浏览器、GUI 应用、或不同内核行为 → **Virtual Machine**（加 `--vm`）

## 测试工作流

### 1. 查找测试镜像

Incus 内置 `images:` 远程服务器，提供数千种系统镜像（各种 Linux
发行版、版本、架构），非常适合做兼容性矩阵测试。

```bash
# 查看可用的远程服务器
incus remote list

# 列出 images: 远程上的所有可用镜像（数千个）
incus image list images:

# 按名称过滤（例如查找 Debian 镜像）
incus image list images: debian

# 按发行版 + 版本 + 架构组合过滤
incus image list images: ubuntu 24.04 arm64

# 按属性过滤
incus image list images: ubuntu architecture=x86_64

# 查看镜像详情（别名或指纹）
incus image info images:ubuntu/24.04
incus image show images:ubuntu/24.04

# 查看特定属性（如版本号）
incus image get-property images:debian/12 release
```

**镜像命名规则**：`<remote>:<distro>/<release>[/<variant>][/arch]`

- `images:ubuntu/24.04` → Ubuntu 24.04 LTS, x86_64
- `images:debian/12` → Debian 12, x86_64
- `images:alpine/3.21` → Alpine 3.21, x86_64
- `images:rocky/9` → Rocky Linux 9, x86_64
- `images:ubuntu/24.04/arm64` → Ubuntu 24.04 LTS, arm64
- `images:debian/12/cloud` → Debian 12 cloud variant

> 测试建议：优先使用轻量镜像（Alpine、Debian）做快速冒烟测试，
> 完整发行版（Ubuntu、Fedora）做兼容性测试。

### 2. 创建隔离的测试环境

#### 创建一次性测试容器（最常用）

```bash
# 创建并启动一个干净的测试容器
incus launch images:debian/12 test-run

# 临时容器（停止后自动删除，适合 CI 或一次性测试）
incus launch images:alpine/3.21 test-quick --ephemeral

# 指定资源限制（模拟低配环境测试）
incus launch images:debian/12 test-limited \
  --config limits.cpu=1 \
  --config limits.memory=256MiB \
  --config limits.disk=2GiB

# 指定存储池和网络（隔离网络测试）
incus launch images:ubuntu/24.04 test-net \
  --storage test-pool \
  --network test-bridge

# 仅创建不启动（准备好环境，测试时再启动）
incus init images:ubuntu/24.04 test-ready
incus start test-ready
```

#### 创建虚拟机（浏览器/UI 测试）

```bash
# 加 --vm 即为虚拟机（适合浏览器自动化测试）
incus launch images:ubuntu/24.04 test-vm --vm \
  --config limits.cpu=2 \
  --config limits.memory=4GiB \
  --device root,size=20GiB
```

#### 创建应用容器（OCI/Docker 服务冒烟测试）

```bash
# 先添加 OCI 远程（如 Docker Hub）
incus remote add oci-docker https://docker.io --protocol=oci

# 从 OCI 镜像创建容器做冒烟测试
incus launch oci-docker:nginx test-nginx --ephemeral

# 运行一次性服务，测试其行为
incus launch oci-docker:hello-world --ephemeral --console
```

> **AI 智能体注意**：默认创建的是系统容器。需要虚拟机时务必加 `--vm`。

### 3. 管理测试实例

```bash
# 列出所有测试实例
incus list

# 按类型/状态过滤
incus list type=container
incus list type=virtual-machine
incus list status=running
incus list status=stopped

# 按名称搜索（支持正则）
incus list test-.*

# 查看更多列（自定义列）
incus list -c nFs46,config:image.os

# 查看实例详细信息（IP 地址、资源使用）
incus info test-run

# 查看实例配置
incus config show test-run
incus config show test-run --expanded  # 包含 profile 展开后的配置
```

### 4. 在测试环境中执行命令

`incus shell` 是 `incus exec` 的别名，自动以 root 登录：

```bash
# 登录到测试环境（交互式，适合调试测试）
incus shell test-run
# 相当于: incus exec test-run -- su -l

# 执行单条测试命令
incus exec test-run -- python -m pytest tests/

# 执行带管道的命令（需通过 shell）
incus exec test-run -- sh -c "ps aux | grep python"

# 指定工作目录
incus exec test-run --cwd /project -- make test

# 设置环境变量（传递测试配置）
incus exec test-run --env TEST_DB=localhost --env CI=true -- make test

# 非交互模式（适合 CI 脚本）
incus exec test-run -T -- cat /test-results.xml

# 从 stdin 传入测试数据
echo '{"test":"data"}' | incus exec test-run -- tee /tmp/input.json

# 复制文件（上传测试脚本、拉取测试结果）
incus file push ./test-suite.sh test-run/tmp/
incus file pull test-run/tmp/test-results.xml ./
```

> **AI 智能体注意**：
> - `incus exec` 不适合交互式程序（如 vim/htop），需要用 `incus shell`
> - 对虚拟机需要安装 incus-agent 才能使用 `incus exec` 和 `incus file`

### 5. 测试状态管理：快照与回滚

这是测试场景中最核心的特性之一。通过快照可以在每次测试后快速
恢复到已知的干净状态，避免测试间相互污染。

```bash
# 测试前创建快照（打一个干净的基线）
incus snapshot create test-run clean-base

# 执行测试...
incus exec test-run -- make test

# 测试后恢复到干净状态（比重新创建快得多）
incus snapshot restore test-run clean-base

# 创建多个快照做不同测试场景
incus snapshot create test-run pre-config-test
incus exec test-run -- ./configure --option=a
incus snapshot create test-run config-a
incus exec test-run -- ./configure --option=b
incus snapshot create test-run config-b

# 在配置间切换测试
incus snapshot restore test-run config-a
incus exec test-run -- make test

incus snapshot restore test-run config-b
incus exec test-run -- make test

# 列出所有快照
incus snapshot list test-run

# 查看快照配置
incus snapshot info test-run clean-base

# 删除不需要的快照
incus snapshot delete test-run old-snapshot
```

> **测试要点**：快照恢复是秒级操作，远快于从头创建容器 + 安装依赖。
> 在 CI 中可以将模板环境预创建好，打快照，每次 CI 运行前恢复。

### 6. 重启测试环境

```bash
# 普通重启
incus restart test-run

# 强制重启（快速重置卡住的测试环境）
incus restart test-run --force

# 设置超时（等待 N 秒后强制）
incus restart test-run --timeout 30

# 重启并查看启动日志
incus restart test-run --console

# 重启所有实例
incus restart --all
```

### 7. 回收测试环境

```bash
# 停止测试实例
incus stop test-run

# 强制停止
incus stop test-run --force

# 删除测试实例（前提：实例已停止）
incus delete test-run

# 强制删除运行中的实例
incus delete test-run --force   # 等价于 stop + delete

# 删除前确认提示
incus alias add delete "delete -i"  # 设置为每次确认
incus delete test-run               # 会提示确认

# 保护重要测试环境不被误删
incus config set test-run security.protection.delete true
```

```{caution}
`incus delete` 永久删除实例及其所有快照，不可恢复。
测试环境建议用完即删，保持宿主机干净。
```

## 测试场景端到端示例

### 场景一：冒烟测试 — 验证应用在干净环境中的行为

```bash
# 1. 从预构建的测试镜像创建临时容器
incus launch images:alpine/3.21 smoke-test --ephemeral

# 2. 安装被测应用
incus exec smoke-test -- apk add myapp

# 3. 执行冒烟测试
incus exec smoke-test -- myapp --version
incus exec smoke-test -- myapp --help
incus exec smoke-test -- myapp quick-test

# 4. 验证输出
incus exec smoke-test -- echo "Smoke test passed"

# 5. 停止后容器自动删除（--ephemeral）
incus stop smoke-test
```

### 场景二：集成测试 — 多容器协作

```bash
# 1. 创建数据库容器
incus launch images:ubuntu/24.04 test-db
incus exec test-db -- apt update
incus exec test-db -- apt install -y postgresql

# 2. 创建应用容器，连接到数据库
incus launch images:ubuntu/24.04 test-app
incus exec test-app -- apt update
incus exec test-app -- apt install -y curl

# 3. 获取数据库容器 IP
DB_IP=$(incus info test-db | grep inet | head -1 | awk '{print $2}')

# 4. 在应用容器中执行集成测试
incus exec test-app --env DB_HOST=$DB_IP -- make integration-test

# 5. 清理
incus stop test-db test-app
incus delete test-db test-app
```

### 场景三：兼容性矩阵测试 — 多发行版并行验证

```bash
# 1. 同时启动多个发行版
for distro in ubuntu/24.04 debian/12 alpine/3.21 rocky/9; do
  name="compat-$(echo $distro | tr '/' '-')"
  incus launch images:$distro $name
done

# 2. 在所有环境中并行执行测试
for container in compat-ubuntu-24.04 compat-debian-12 compat-alpine-3.21 compat-rocky-9; do
  incus exec $container -T -- make test &
done
wait

# 3. 收集测试结果
for container in compat-ubuntu-24.04 compat-debian-12 compat-alpine-3.21 compat-rocky-9; do
  incus file pull $container/tmp/test-results.xml ./results-$container.xml
done

# 4. 一次性清理
incus stop --all
incus list --format csv | grep compat- | cut -d, -f1 | xargs incus delete
```

### 场景四：数据库功能测试 — 快照回滚实现测试隔离

```bash
# 1. 准备测试数据库容器
incus launch images:ubuntu/24.04 test-mysql
incus exec test-mysql -- apt update
incus exec test-mysql -- apt install -y mysql-server
incus exec test-mysql -- systemctl start mysql

# 2. 导入测试数据
incus exec test-mysql -- mysql -u root < /tmp/setup.sql

# 3. 打基线快照
incus snapshot create test-mysql baseline

# 4. 运行测试组 A（修改数据库状态）
incus exec test-mysql -- mysql -u root -e "INSERT INTO ..."
incus exec test-app --env DB_HOST=$DB_IP -- make test-group-a

# 5. 恢复到基线，运行测试组 B（互不污染）
incus snapshot restore test-mysql baseline
incus exec test-app --env DB_HOST=$DB_IP -- make test-group-b

# 6. 清理
incus stop test-mysql test-app
incus delete test-mysql test-app
```

### 场景五：浏览器自动化测试（虚拟机 + GUI）

```bash
# 1. 创建 VM（浏览器测试需要完整 GUI 栈）
incus launch images:ubuntu/24.04 test-browser --vm \
  --config limits.cpu=2 \
  --config limits.memory=4GiB

# 2. 安装浏览器和测试工具
incus exec test-browser -- apt update
incus exec test-browser -- apt install -y firefox chromium-browser

# 3. 安装浏览器驱动（Selenium/Playwright）
incus exec test-browser -- npm install -g playwright
incus exec test-browser -- npx playwright install-deps

# 4. 上传测试脚本并执行
incus file push ./browser-tests.js test-browser/tmp/
incus exec test-browser -- node /tmp/browser-tests.js

# 5. 拉取测试报告
incus file pull test-browser/tmp/test-report.html ./

# 6. 清理
incus stop test-browser
incus delete test-browser
```

### 场景六：CI 流水线集成

```bash
# CI 脚本模板（适合 GitLab CI / GitHub Actions）
# 1. 检查是否有预热的测试容器
if ! incus info ci-runner &>/dev/null; then
  # 2. 创建干净的测试环境
  incus launch images:debian/12 ci-runner
  incus exec ci-runner -- apt update
  incus exec ci-runner -- apt install -y build-essential git

  # 3. 安装依赖后打快照作为 CI 基线
  incus snapshot create ci-runner ci-base
fi

# 4. 每次 CI 运行前恢复到基线
incus snapshot restore ci-runner ci-base

# 5. 拉取最新代码并执行测试
incus exec ci-runner -- git clone $REPO /project
incus exec ci-runner --cwd /project -- make test

# 6. 收集测试结果
incus file pull ci-runner/project/test-results.xml ./
```

## 测试最佳实践

### 环境隔离

- 每个测试用例使用独立的容器，避免状态污染
- 使用 `--ephemeral` 确保容器停止即销毁
- 多组并行测试使用不同命名前缀（如 `test-${JOB_ID}-`）

### 状态管理

- **快照是测试工程师最好的朋友**：执行状态修改的操作前先打快照
- 每次测试结束后恢复到已知的干净状态，而不是重装
- 将基础环境（OS + 依赖）预创建好并打快照，节省 CI 时间

### 资源控制

- 使用 `limits.cpu` / `limits.memory` 模拟低配环境，验证应用在资源受限下的表现
- 使用 `limits.disk` 测试磁盘空间不足时的错误处理
- 使用独立网络隔离测试流量，避免影响宿主机网络

### 并行测试

- Incus 容器轻量，单台宿主机可并行运行数百个容器
- 每个并行任务使用独立容器命名，避免冲突
- 批量操作时注意 incus daemon 的连接数限制

### 调试技巧

- `incus info <instance>` 查看 IP、资源使用、进程列表
- `incus exec <instance> --cwd /path -- command` 在指定目录执行命令
- `incus file push/pull` 快速传输测试脚本和结果
- `incus snapshot create` 在调试前打快照，搞砸了秒级恢复

## 注意事项

- **存储**：初始化时自动创建默认存储池（通常为 `default` 或 `incus`）。
  可通过 `incus storage list` 和 `incus storage info <pool>` 查看
- **网络**：默认创建 `incusbr0` 网桥，实例自动获取 DHCP IP。
  用 `incus network list` 和 `incus network show incusbr0` 查看
- **镜像缓存**：拉取过的镜像会缓存在本地，`incus image list` 可查看已缓存镜像。
  用 `incus image delete <fingerprint>` 清理
- **Profile**：实例配置通过 profile 继承。查看默认 profile：
  `incus profile show default`
- **权限**：普通用户需在 `incus` 组中才能访问 unix socket。
  操作报 "permission denied" 时检查 `groups $USER`
- **快照**：操作前打快照是好习惯：
  `incus snapshot create <instance> <snapshot-name>`
  恢复：`incus snapshot restore <instance> <snapshot-name>`
- **远程**：`incus remote add` 可添加其他 Incus 服务器或 OCI 镜像仓库
