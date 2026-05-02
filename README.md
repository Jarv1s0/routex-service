# routex-service

`routex-service` is the RouteX system service component, forked from upstream `sparkle-service`.

This repository builds two runtime binaries:

- `routex-service`: the main background service.
- `routex-run.exe`: the Windows helper used by scheduled tasks to launch RouteX with elevated privileges.

Runtime defaults:

- CLI command: `routex-service`
- Windows named pipe: `\\.\pipe\routex\service`
- Unix socket: `/tmp/routex-service.sock`

## Build

```bash
go build -o routex-service .
```

## Service Commands

```bash
routex-service service install
routex-service service uninstall
routex-service service start
routex-service service stop
routex-service service restart
routex-service service status
```

## Sysproxy Commands

Upstream moved system proxy operations under the `sysproxy` subcommand.

```bash
routex-service sysproxy proxy -s 127.0.0.1:7890 -b "localhost;127.*;10.*;192.168.*"
routex-service sysproxy pac -u http://127.0.0.1:7890/pac
routex-service sysproxy disable
routex-service sysproxy status
```

## HTTP API

The service listens on the local named pipe or Unix socket and exposes routes for:

- `/ping`
- `/core`
- `/sysproxy`
- `/sys`
- `/service`

Protected APIs use the upstream Ed25519 request signing flow plus OS-level caller identity checks.

RouteX-specific key storage defaults to:

- Windows: `C:\ProgramData\routex\keys`
- Unix-like systems: `<config-dir>/routex/keys`
