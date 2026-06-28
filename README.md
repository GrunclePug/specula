# specula

specula - A minimalist, Unix-inspired ping status monitor and telemetry dashboard.

## Overview

`specula` is a lightweight, hypermedia-driven status monitoring dashboard designed to track target hosts using ICMP pings or TCP dial checks. Built with Go and HTMX, it compiles into a single binary with embedded web assets, making it exceptionally easy to host locally, via Docker Compose, or inside lightweight Kubernetes clusters.

## Features

* **Zero-CGO Pure Go**: Statically compiled using Go's standard library `http.ServeMux` and a CGO-free SQLite driver (`modernc.org/sqlite`) for absolute portability.
* **Hypermedia-Driven UI**: Powered by HTMX to perform dynamic target status updates, dashboard filtering, and target/group management inline without JavaScript framework overhead.
* **Self-Contained Executable**: Embeds all HTML templates and static scripts directly into the binary via `go:embed`, enabling a true single-file deployment.
* **Hybrid Check Engine**: Automatically switches ping strategies: targets with a port suffix (e.g. `localhost:8080`) are checked using a TCP dial, whereas targets without a port (e.g. `google.com`) use system ICMP ping checks.
* **Telemetry & Metrics Shipping**: Supports real-time local storage (SQLite) or optional remote metrics shipping to InfluxDB via InfluxDB Line Protocol.
* **Dynamic Grouping**: Organize targets into custom groups (e.g., `web`, `dns`, `public`, `infrastructure`) and filter statuses dynamically via the HTMX-driven dashboard.
* **No-Restart Workflow**: Easily add, edit, or delete monitored targets and groups directly in the browser dashboard without reloading or touching files.
* **Container-Ready**: Minimalist multi-stage `Dockerfile` compilation running on a Distroless static runtime image with host-bound database volume mounts.

## Usage

You can run the application directly from the compiled binary or deploy it via container stacks.

### Local Execution

```bash
# Copy and configure environment variables
cp .env.example .env

# Build the Go executable
make build

# Run the local dashboard server (defaults to port 8080)
./bin/specula
```

### Docker Compose

```bash
# Start the container stack in the background (binds to host port 10001)
docker compose up -d

# View logs
docker compose logs -f

# Shut down the stack
docker compose down
```

### Requirements

* Go 1.26+
* Make
* Docker / Docker Compose (optional for container hosting)

### Configuration

Customize options inside your local `.env` file:
```env
# Specula Web Server Configuration
SPECULA_ADDR=:8080
SPECULA_PING_INTERVAL=5s
SPECULA_PING_TIMEOUT=2s
SPECULA_DB_PATH=data/specula.db

# Comma-separated list of targets to monitor.
SPECULA_TARGETS=google.com,github.com,cloudflare.com,8.8.8.8

# InfluxDB Configuration (Enabled by running the container with command flag: -influx)
INFLUX_URL=http://localhost:8086
INFLUX_TOKEN=your-token-here
INFLUX_ORG=your-organization-name
INFLUX_BUCKET=your-bucket-name
```

### Maintenance

Clean build artifacts:
```bash
make clean
```

## Contributing

Contributions are welcome! If you'd like to contribute to this project, please fork the repository and submit a pull request.

## Author

Chad Humphries |
[Website](https://grunclepug.com/) |
[GitHub Profile](https://github.com/GrunclePug)

## Other Projects

Check out some of my other projects on GitHub: [Here](https://github.com/GrunclePug?tab=repositories)
