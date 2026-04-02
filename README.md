# DiskSage

DiskSage 是一个基于 Go + React 的智能磁盘清理助手，目标是通过目录扫描与智能分析生成可执行的清理建议。

当前仓库已包含：
- Go 后端核心模块（扫描、分析、清理、权限、配置）
- React 前端页面与组件
- 单元测试与基础静态校验链路

## 技术栈

- Go 1.21+
- Node.js 18+（建议 LTS）
- React 18 + TypeScript + Vite
- Wails v2（配置文件已准备）

## 项目结构

```text
DiskSage/
├── main.go
├── app.go
├── wails.json
├── internal/
│   ├── scanner/
│   ├── analyzer/
│   ├── cleaner/
│   ├── privilege/
│   ├── config/
│   └── models/
├── frontend/
│   ├── package.json
│   ├── index.html
│   └── src/
├── build/windows/
└── TASK_PROGRESS.md
```

## 环境准备

### 1. 安装 Go

确认版本：

```powershell
go version
```

需要 `go1.21` 或更高。

### 2. 安装 Node.js

确认版本：

```powershell
node -v
npm -v
```

建议 Node.js 18 或更高。

### 3. （可选）安装 Wails CLI

如果你要走 Wails 桌面打包流程：

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

## 快速开始

### 方式 A：后端快速验证（Go）

在项目根目录执行：

```powershell
go run .
```

当前 `main.go` 已切换为 Wails 正式启动入口，可直接用于桌面应用运行。

### 方式 B：前端独立开发（Vite）

```powershell
cd frontend
npm install
npm run dev
```

启动后访问 Vite 输出的本地地址（默认通常是 `http://localhost:34115`）。

说明：前端在未接入 Wails Runtime 时会走本地 mock 回退数据，方便独立联调 UI。

## 测试与校验

在项目根目录执行：

### 1. Go 单元测试

```powershell
go test ./...
```

### 2. Go 静态检查

```powershell
go vet ./...
```

### 3. 前端构建检查

```powershell
cd frontend
npm install
npm run build
```

构建产物在 `frontend/dist`。

## 构建发布

### 1. 当前仓库可执行构建（前后端分别构建）

后端编译：

```powershell
go build -o DiskSage.exe .
```

前端编译：

```powershell
cd frontend
npm run build
```

### 2. Wails 构建

仓库已包含 `wails.json`，且已接入 Wails `wails.Run(...)` 入口，可使用：

```powershell
wails dev
wails build
```

## 常见命令速查

```powershell
# 根目录
go test ./...
go vet ./...
go run .

# 前端
cd frontend
npm install
npm run dev
npm run build
npm run preview
```

## 常见问题

### 1. `npm run build` 报入口错误

请确认 `frontend/index.html` 存在，并且 `src/main.tsx` 路径正确。

### 2. 清理命令执行失败

`internal/cleaner/strategy.go` 中对命令执行有白名单限制，只允许安全前缀命令。

### 3. 需要管理员权限

系统目录清理可能需要提权，逻辑见 `internal/privilege`。

## 后续建议

- 增加真实 LLM Provider 的端到端调用回归测试
- 增加 Windows 回收站 API 适配，替代当前临时回收目录策略
