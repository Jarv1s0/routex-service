# routex-service

此仓库现在同时构建两个二进制产物：

- `routex-service`：主服务程序构建产物
- `routex-run.exe`：Windows 辅助启动器构建产物

运行时默认命名如下：

- CLI 命令名：`routex-service`
- Windows 命名管道：`\\.\pipe\routex\service`
- Unix Socket：`/tmp/routex-service.sock`
