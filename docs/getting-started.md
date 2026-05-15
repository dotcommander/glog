# Getting Started

This is the shortest path from a fresh machine to a working local GLog server.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/dotcommander/glog/main/scripts/install.sh | sh
glog doctor
```

If the installer says the install directory is not on `PATH`, run the `export PATH=...`
line it prints, then run `glog doctor` again.

## Start The Server

```bash
glog serve
```

Open `http://localhost:6016` for the dashboard.

If you are running from a source checkout and have `just` installed, this also
builds the dashboard first:

```bash
just serve
```

## Register This Machine

In another terminal:

```bash
glog host register --server http://localhost:6016 --name my-machine
```

This writes `./glog.json` with the server URL and API key. Future `glog log`
commands use that file automatically.

## Send A Test Log

```bash
glog log "hello from glog" --field source=first-run
```

## Troubleshoot

```bash
glog doctor
curl http://localhost:6016/health
```

Common first-run fixes:

- `glog: command not found`: add the install directory printed by the installer to `PATH`.
- `Config: missing`: run `glog host register --server http://localhost:6016 --name my-machine`.
- `Server: unreachable`: start the server with `glog serve`, or pass the right URL with `--server`.

## Run GLog On Login On macOS

From a source checkout, the easiest path is the `Justfile`:

```bash
just service-install
just service-status
just health
```

This installs a macOS `launchd` user agent named `dev.glog` that:

- runs `/Users/vampire/go/bin/glog serve`
- serves the dashboard from `web/build`
- stores the database at `./glog.db`
- restarts automatically with `KeepAlive`
- starts automatically at login with `RunAtLoad`

Useful service commands:

```bash
just service-restart
just service-logs
just service-uninstall
```

Override the listen address for local testing:

```bash
GLOG_ADDR=127.0.0.1:6020 just service-install
```
