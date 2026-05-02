# routex-service

`routex-service` 是 RouteX 的系统服务组件，基于上游 `sparkle-service` 改造，用于在本机后台提供 RouteX 相关的服务控制、核心进程控制、系统代理和系统能力接口。

该仓库当前包含两个运行时二进制：

- `routex-service`：主服务程序，也是命令行入口。
- `routex-run.exe`：Windows 辅助程序，用于由计划任务以提权方式启动 RouteX。

## 功能范围

- 注册、启动、停止、重启和查询 RouteX 系统服务。
- 初始化服务端认证配置，绑定客户端公钥和允许访问服务的系统身份。
- 管理系统代理：普通代理、PAC、禁用代理和状态查询。
- 通过本地命名管道或 Unix socket 暴露 HTTP API。
- 对受保护 API 执行 Ed25519 请求签名校验和系统调用方身份校验。

## 运行时默认值

- CLI 命令：`routex-service`
- Windows 命名管道：`\\.\pipe\routex\service`
- Unix socket：`/tmp/routex-service.sock`
- Windows 密钥目录：`C:\ProgramData\routex\keys`
- Unix-like 密钥目录：`<config-dir>/routex/keys`

## 构建

构建主服务：

```bash
go build -o routex-service .
```

构建 Windows runner：

```bash
go build -o routex-run.exe ./cmd/runner
```

项目模块声明为 `go 1.25.0`，实际构建环境需要满足 `go.mod` 中的版本和依赖要求。

## 服务命令

安装并启动服务：

```bash
routex-service service install
```

管理服务：

```bash
routex-service service start
routex-service service stop
routex-service service restart
routex-service service status
routex-service service uninstall
```

以前台方式运行服务：

```bash
routex-service service run
```

初始化认证配置：

```bash
routex-service service init --public-key <public-key> --authorized-sid <windows-sid>
routex-service service init --public-key <public-key> --authorized-uid <unix-uid>
```

说明：

- `--public-key` 必填，用于保存客户端 Ed25519 公钥。
- `--authorized-sid` 用于 Windows，绑定允许访问服务的 SID。
- `--authorized-uid` 用于 Unix-like 系统，绑定允许访问服务的 UID。
- 如果服务正在运行且配置有变化，`service init` 会尝试重启服务使配置生效。

## Sysproxy 命令

设置普通代理：

```bash
routex-service sysproxy proxy -s 127.0.0.1:7890 -b "localhost;127.*;10.*;192.168.*"
```

设置 PAC：

```bash
routex-service sysproxy pac -u http://127.0.0.1:7890/pac
```

禁用代理并查询状态：

```bash
routex-service sysproxy disable
routex-service sysproxy status
```

通用网络设备参数：

```bash
routex-service --device <name> sysproxy status
routex-service --only-active-device sysproxy status
routex-service --use-registry sysproxy status
```

## 测试用服务入口

仓库内还有一个测试用途的 `server` 子命令：

```bash
routex-service server
```

该命令会监听 `127.0.0.1:10002`，主要用于本地调试，不是独立的 `routex-server` 二进制。

## HTTP API

服务通过本地监听地址提供 HTTP API。默认监听地址可通过 `--listen` 覆盖：

```bash
routex-service --listen <addr> service run
```

公开接口：

- `GET /ping`

受保护接口需要通过 Auth V2 请求签名，并通过系统调用方身份校验：

- `GET /test`
- `/core`
- `/sysproxy`
- `/sys`
- `/service`

当前路由包含：

- `GET /core`
- `GET /core/events`
- `GET /core/profile`
- `POST /core/profile`
- `POST /core/start`
- `POST /core/stop`
- `POST /core/restart`
- `GET /sysproxy/status`
- `GET /sysproxy/events`
- `POST /sysproxy/pac`
- `POST /sysproxy/proxy`
- `POST /sysproxy/disable`
- `POST /sys/dns/set`
- `POST /service/stop`
- `POST /service/restart`

## 目录结构

- `cmd/`：CLI 命令和系统服务生命周期入口。
- `cmd/runner/`：Windows `routex-run.exe` 辅助程序。
- `route/`：HTTP 路由、认证中间件和各 API 模块。
- `service/`：跨平台服务注册与运行封装。
- `listen/`：本地命名管道和 socket 监听实现。
- `core/`：RouteX 核心进程相关能力。
- `sys/`：系统能力接口。
- `log/`：日志初始化和输出封装。

## 许可证

本项目使用 `GPL-3.0` 许可证，详见 `LICENSE`。
