set dotenv-load := true

repo := justfile_directory()
home := env_var("HOME")
uid := `id -u`
label := "dev.glog"
plist := home + "/Library/LaunchAgents/" + label + ".plist"
bin := home + "/go/bin/glog"
db := repo + "/glog.db"
web := repo + "/web/build"
addr := env_var_or_default("GLOG_ADDR", "127.0.0.1:6016")

default:
    @just --list

# Build the static dashboard.
build-web:
    cd web && bun run build

# Install the current CLI to the active GOPATH bin directory.
install-cli:
    go install ./cmd/glog

# Build dashboard and install the CLI binary.
build: build-web install-cli

# Run all backend tests.
test:
    go test ./... -timeout 5m

# Run the frontend production build.
test-web:
    cd web && bun run build

# Run the local server in the foreground.
serve: build-web
    {{bin}} serve --addr {{addr}} --db {{db}} --web {{web}}

# Check the installed CLI and local server configuration.
doctor:
    {{bin}} doctor --server http://{{addr}}

# Open the dashboard in the default browser.
dashboard:
    open "http://{{addr}}"

# Install and start the macOS launchd user service.
service-install: build
    mkdir -p "{{home}}/Library/LaunchAgents"
    printf '%s\n' \
      '<?xml version="1.0" encoding="UTF-8"?>' \
      '<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">' \
      '<plist version="1.0">' \
      '  <dict>' \
      '    <key>Label</key>' \
      '    <string>{{label}}</string>' \
      '    <key>ProgramArguments</key>' \
      '    <array>' \
      '      <string>{{bin}}</string>' \
      '      <string>serve</string>' \
      '      <string>--addr</string>' \
      '      <string>{{addr}}</string>' \
      '      <string>--db</string>' \
      '      <string>{{db}}</string>' \
      '      <string>--web</string>' \
      '      <string>{{web}}</string>' \
      '    </array>' \
      '    <key>WorkingDirectory</key>' \
      '    <string>{{repo}}</string>' \
      '    <key>RunAtLoad</key>' \
      '    <true/>' \
      '    <key>KeepAlive</key>' \
      '    <true/>' \
      '    <key>StandardOutPath</key>' \
      '    <string>{{repo}}/glog.launchd.log</string>' \
      '    <key>StandardErrorPath</key>' \
      '    <string>{{repo}}/glog.launchd.err.log</string>' \
      '  </dict>' \
      '</plist>' > "{{plist}}"
    plutil -lint "{{plist}}"
    launchctl bootout gui/{{uid}}/{{label}} 2>/dev/null || true
    launchctl bootstrap gui/{{uid}} "{{plist}}"
    launchctl enable gui/{{uid}}/{{label}}
    launchctl kickstart -k gui/{{uid}}/{{label}}
    @echo "Installed {{label}}. Dashboard: http://{{addr}}"

# Stop and remove the macOS launchd user service.
service-uninstall:
    launchctl bootout gui/{{uid}}/{{label}} 2>/dev/null || true
    rm -f "{{plist}}"

# Restart the macOS launchd user service.
service-restart:
    launchctl kickstart -k gui/{{uid}}/{{label}}

# Show launchd status for the GLog service.
service-status:
    launchctl print gui/{{uid}}/{{label}}

# Tail launchd stdout/stderr logs.
service-logs:
    tail -f "{{repo}}/glog.launchd.log" "{{repo}}/glog.launchd.err.log"

# Check the running service health endpoint.
health:
    curl -fsS "http://{{addr}}/health?detailed=true"
