# GitDesk

一个独立的 **Git 桌面工作台**，用 Go + Wails 把 Markdown 编辑器里的 Git 功能抽出来单独做成应用。单个 `GitDesk.exe`，无需 Electron、无需额外运行时前端服务，目前只提供 **Windows x64** 版本。

界面分为「启动页」和「工作台」两部分：启动页负责选目录和最近打开记录，工作台提供同步、改动、分支、历史四个视图。

## 功能

**打开仓库**

- 系统目录选择对话框
- 把文件夹拖进窗口；拖入仓库内**任意文件**会自动向上定位到 `.git` 根目录
- 命令行传目录：`GitDesk.exe D:\repos\my-project`
- 把文件夹直接拖到 exe 图标上启动
- 普通目录可一键 `git init` 初始化为仓库

**最近打开**

- 最多 12 条，显示路径与打开时间，支持单条移除、清空
- 自动过滤已删除 / 不再是仓库的目录
- 持久化在 `%APPDATA%\GitDesk\recent.json`

**Git 操作**

| 视图 | 能力 |
| --- | --- |
| 同步 | 分支、远端、上游、领先/落后、冲突、合并中状态；拉取、推送、刷新 |
| 改动 | 按目录树勾选文件、填写提交说明、**提交并推送**；冲突时选择保留本地 / 远程 / 放弃合并 |
| 分支 | 本地分支列表、新建、切换、删除（未合并分支需二次确认，只删本地不删远端） |
| 历史 | 提交日志分页加载 |

**其他**

- Git 可执行文件自动探测（PATH + 常见安装位置），设置里可手动指定并重新检测
- 明暗主题、在资源管理器中显示、操作日志、Toast 提示
- 快捷键：`F5` 刷新状态，`Ctrl+O` 返回启动页，`Esc` 关闭弹窗

## 直接运行

从 [Releases](https://github.com/ggbao666/git-desk/releases) 下载 `GitDesk.exe` 双击即可。每次推送到 `main` 都会更新固定的 [latest Release](https://github.com/ggbao666/git-desk/releases/tag/latest)，可以用这个链接一直拿到最新构建；正式版本请下载 `v*` 标签的 Release。

启动页是紧凑的小窗口（440 × 620），选择仓库后窗口自动放大为工作台（1120 × 760），点返回又会缩回小窗口。

运行前置条件：

- Windows 10 / 11，且已安装 **WebView2 Runtime**（Win10/11 和 Edge 一般已自带）
- 本机已安装 **Git for Windows**；应用不自带 git，只是调用 `git.exe`

配置与历史记录存放在 `%APPDATA%\GitDesk\` 下的 `settings.json` 和 `recent.json`。

## 从源码构建

需要 **Go 1.26**、**Node.js 22+**、**Wails CLI v2.17.1**、Git，以及 Windows 环境。

```powershell
# 1. 安装 Wails CLI（确保 $(go env GOPATH)\bin 在 PATH 中）
go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1

# 2. 构建前端（main.go 用 go:embed 内嵌 frontend/dist，必须先生成）
cd frontend
npm ci
npm run build
cd ..

# 3. 打包
wails build
```

产物：`build\bin\GitDesk.exe`（约 11 MB，已内嵌图标与前端资源）。

开发调试与测试：

```powershell
wails dev        # 热更新开发模式
go vet ./...
go test ./...    # 在临时 git 仓库上跑完整引擎集成测试
```

## 应用图标

紫色渐变圆角方块 + 白色 Git 分支图（主干两个提交节点、一条特性分支），与启动页左上角 logo 一致。设计源码是 `scripts/generate_icon.py`，用 Pillow 绘制，可随时重新生成：

```powershell
pip install pillow
python scripts/generate_icon.py
```

产物：`build/windows/icon.ico`（16 / 24 / 32 / 48 / 64 / 128 / 256 多尺寸，打包时嵌入 exe）和 `build/appicon.png`（512 × 512）。

## 项目结构

```
main.go              窗口启动、启动参数、拖拽与主题配置
app.go               Wails 绑定：目录选择、拖拽归一、初始化仓库、设置
git_api.go           Git 操作的绑定入口
git_exec.go          git.exe 探测、命令执行、超时与 Windows 隐藏控制台
git_ops.go           status / pull / push / commit / 冲突 / 分支 / 历史
git_types.go         与前端约定的数据结构
recent.go            最近打开记录（最多 12 条）
settings.go          设置持久化
frontend/index.html  界面结构
frontend/src/        启动页、工作台、样式、API 封装
frontend/wailsjs/    Wails 自动生成的绑定（已入库，clone 后可直接开发）
build/windows/       图标、版本信息、manifest
scripts/generate_icon.py   应用图标设计与生成脚本
.github/workflows/build-windows.yml   Windows 自动打包工作流
```

## CI / 自动打包

`.github/workflows/build-windows.yml` 只跑 `windows-latest`，产出 Windows amd64 单文件 exe：

- **触发**：push 到 `main`、针对 `main` 的 PR、手动运行、以及推送 `v*` 标签
- **流程**：setup-go（读 `go.mod` 的 Go 版本）+ setup-node 22（npm 缓存）→ `npm ci` → 安装 Wails CLI → `wails build`（按 `wails.json` 内部执行 `npm run build` 并生成绑定；`CGO_ENABLED=0`，纯 Go 编译，不需要 gcc）
- **产物**：每次运行上传 Artifact `GitDesk-Windows-x64`
- **发版**：推送 `v*` 标签时自动创建 GitHub Release 并附上 `GitDesk.exe`（重跑同一标签会改为覆盖上传，不会失败）
- **滚动构建**：每次 push 到 `main` 额外更新固定标签 `latest` 的 Release，覆盖上传最新 exe，便于用固定链接下载开发版

手动发布一个版本：

```powershell
git tag v1.0.0
git push origin v1.0.0
```

## 说明

- 本项目**不打包 macOS / Linux**，也暂不提供安装程序，只有免安装的单文件 exe。
- 首次在某目录使用时，建议先在设置中确认 Git 路径探测结果；若本机有多个 Git（例如便携版），可手动指定。
