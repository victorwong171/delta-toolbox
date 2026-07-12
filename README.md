# Delta Toolbox (Monorepo)

`delta-toolbox` 是一个基于 **Go Workspaces (Go 工作区)** 模式组织的 Monorepo 仓库，聚合了多个独立工具链与服务子项目。通过将零散项目进行大仓化（Monorepo）聚合管理，便于统一工程规范、依赖治理和后续公共类库的沉淀。

---

## 📂 目录结构说明

```text
/delta-toolbox
├── packages/               # 工具链与服务子项目目录
│   ├── ncm-dumper/         # NCM 格式音频解密转码工具
│   ├── net-inspect/        # 网络连通性与 Clash 延迟诊断工具
│   ├── LFS/                # 大文件存储与传输服务
│   └── game-prioritizer/   # 游戏/进程 CPU 优先级优化工具 (Windows)
├── init_workspace.bat      # Windows 环境一键初始化 Go 工作区脚本
├── init_workspace.sh       # Linux/macOS 环境一键初始化 Go 工作区脚本
├── .gitignore              # 忽略本地开发的 go.work 配置文件
└── LICENSE                 # 许可证
```

---

## 🚀 开发者快速上手

### 1. 克隆仓库与初始化工作区
克隆本项目后，**第一步**必须初始化本地的 Go 工作区配置。
由于工作区配置文件 `go.work` 会因个人的本地路径和调试需求而异，**该文件已被 Git 忽略，切勿提交**。

* **Windows 用户**:
  直接双击运行或在终端执行：
  ```cmd
  init_workspace.bat
  ```
* **Unix-like (macOS/Linux) 用户**:
  给脚本赋予执行权限并运行：
  ```bash
  chmod +x init_workspace.sh
  ./init_workspace.sh
  ```

脚本会自动检测并在根目录下生成 `go.work` 文件，将 `packages/` 下的所有 Go 模块纳入当前工作区中。

### 2. 依赖同步与对齐
在工作区初始化完毕或子项目 `go.mod` 发生变更时，在**根目录**执行以下命令，强制将大仓中各个子项目的依赖版本进行向上对齐：
```bash
go work sync
```

---

## 🛠️ 日常开发指南

### 1. 如何运行/编译子项目
在 Go 工作区模式下，您可以在**大仓根目录**直接指定子包路径进行开发调试：

* **运行 `net-inspect`**:
  ```bash
  go run ./packages/net-inspect
  ```
* **运行 `ncm-dumper` 命令行工具**:
  ```bash
  go run ./packages/ncm-dumper/cmd/ncmdump --help
  ```
* **编译 `LFS` 服务端**:
  ```bash
  go build -o bin/lfs-server.exe ./packages/LFS/cmd/lfs-server
  ```
* **运行 `game-prioritizer`**:
  ```bash
  go run ./packages/game-prioritizer
  ```

### 2. 新增子项目
如果您需要在大仓中新增一个服务或工具包：
1. 在 `packages/` 目录下创建新目录并初始化 `go.mod`：
   ```bash
   mkdir packages/my-new-tool
   cd packages/my-new-tool
   go mod init my_new_tool
   ```
2. 在大仓根目录将新项目注册进本地工作区：
   ```bash
   go work use ./packages/my-new-tool
   ```
3. 执行 `go work sync` 同步依赖。
4. 修改大仓根目录下的 `init_workspace.*` 脚本，将新路径 `./packages/my-new-tool` 追加进去，以便其他团队成员克隆后一键初始化。

### 3. 公共类库引用 (跨模块调用)
如果未来需要在大仓内引入跨项目公共类库（例如 `packages/common`）：
1. 在调用方（如 `packages/net-inspect/go.mod`）中引入公共模块：
   ```go
   require my_monorepo/common v0.0.0
   ```
2. 通过工作区模式，Go 会自动映射到本地的 `packages/common` 路径下，无需在 `go.mod` 中显式书写 `replace` 指令。
3. 记得执行 `go work use ./packages/common` 将公共模块纳入工作区。

---

## 🔄 Git Subtree 持续同步（针对仍需独立维护的子仓）

本项目通过 `git subtree` 聚合了子仓的提交历史。如果在过渡期内仍有外部协作者在原独立仓库提交代码，或者你想将大仓的修改推回原仓库，可参考以下命令：

> **注：如果不打算再维护原独立仓库，可忽略此节，直接在大仓中进行开发提交即可。**

* **拉取子仓的最新改动 (以 ncm-dumper 为例)**:
  ```bash
  git subtree pull --prefix=packages/ncm-dumper <ncm-dumper-repo-url> master --squash
  ```
* **将大仓中的改动推回子仓**:
  ```bash
  git subtree push --prefix=packages/ncm-dumper <ncm-dumper-repo-url> master
  ```
