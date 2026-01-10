# ClaudeChat

一个基于Claude的私有化聊天服务 包含服务端和跨平台客户端 可实现基于调用Claude订阅的Session Key将网页订阅转换为API调用的模式

## 项目结构

```
ClaudeServer/
├── server/          # Go 服务端
├── client/
│   ├── src-wails/   # Wails 桌面端配置
│   └── src-vue/
│       ├── pc/      # PC 客户端 (Vue + Wails)
│       └── mobile/  # 移动端客户端 (Vue + Capacitor)
├── src/             # 配置文件和数据库脚本
├── static/          # 静态网页资源
└── run.ps1          # 构建/运行脚本
```

## 功能特性

- 流式对话响应
- Markdown 渲染
- 多会话管理
- 用量统计与限制
- 设备管理
- MCP支持
- 版本更新检查

## 快速开始

### 环境要求

- Go 1.21+
- Node.js 18+
- Android Studio
- PostgreSQL

### 配置

1. 复制配置模板：
```bash
cp .\server\src\config-template.yaml .\server\src\config.yaml
```

2. 编辑 `config.yaml`，填写必要的认证信息：
   - `organization_id` - Claude 组织 ID
   - `session_key` - Claude 会话密钥

### 运行

使用 PowerShell 脚本：

```powershell
# 启动服务端
.\run.ps1

# 桌面端调试启动
.\run.ps1 -c

# 桌面端编译发布
.\run.ps1 -c -b

# 移动端调试启动
.\run.ps1 -c -m

# 移动端编译发布
.\run.ps1 -c -m -b
```

## 技术栈

### 服务端
- Go
- Gorilla WebSocket
- PostgreSQL

### 桌面端
- Vue 3 + TypeScript
- Wails v3
- Tailwind CSS

### 移动端
- Vue 3 + TypeScript
- Capacitor
- Tailwind CSS

## 许可证

MIT License
