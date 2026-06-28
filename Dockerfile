# Stage 1: Build the Go binary using latest golang:alpine and Makefile
FROM golang:alpine AS builder
WORKDIR /app
# Install make and git to support compiling via the local Makefile
RUN apk add --no-cache make git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Run make build (compiles to bin/specula and reads version from config.mk)
RUN CGO_ENABLED=0 make build

# Stage 2: Extract ping utility and all of its shared library dependencies from Debian
FROM debian:bookworm-slim AS ping-provider
RUN apt-get update && apt-get install -y --no-install-recommends iputils-ping
# Create a folder structure of all dependencies so we copy them cleanly
RUN mkdir -p /ping-deps/bin && \
    cp /bin/ping /ping-deps/bin/ping && \
    for lib in $(ldd /bin/ping | awk '{print $3}' | grep '^/'); do \
        dir="/ping-deps$(dirname "$lib")"; \
        mkdir -p "$dir"; \
        cp -L "$lib" "$dir/"; \
    done

# Stage 3: Final runtime container using Distroless
FROM gcr.io/distroless/base-debian12
WORKDIR /app

# Copy the statically compiled specula binary from the builder stage
COPY --from=builder /app/bin/specula /app/specula

# Copy the ping utility and its dependent shared libraries
COPY --from=ping-provider /ping-deps/ /

# Set runtime environment defaults
ENV SPECULA_ADDR=:8080
ENV SPECULA_DB_PATH=/data/specula.db
ENV SPECULA_JSON_LOG=true
ENV TZ=America/Vancouver

# Expose port 8080 for the HTTP dashboard
EXPOSE 8080

# Run the app
ENTRYPOINT ["/app/specula"]
