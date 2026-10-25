# ---------- Stage 1: build ----------
FROM golang:1.22-alpine AS build

# proxy.golang.org returns 403 from Iran, so we use mirrors.
# "|" (not ",") makes Go fall through to the next entry on ANY error (e.g. 403),
# while "," only falls through on 404/410. "direct" = last resort, fetch from source.
# Override at build time if needed: docker build --build-arg GOPROXY=... .
ARG GOPROXY=https://mirror.abrha.net/repository/go/|https://mirror-go.runflare.com|https://package-mirror.liara.ir/repository/go/|direct
# sum.golang.org may be blocked too; integrity is still checked against the committed go.sum
ARG GOSUMDB=off

WORKDIR /src

# Copy dependency manifests first so this layer is cached until they change
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Static binary (pure-Go MySQL driver, so CGO is not needed)
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/auth ./cmd/main.go

# ---------- Stage 2: runtime ----------
FROM alpine:3.20
RUN addgroup -S app && adduser -S -G app app

WORKDIR /app
COPY --from=build /out/auth ./auth

# The app requires ./envs/dev.env to exist (viper ReadInConfig), and only
# keys present in that file can be overridden by environment variables.
# So we ship a placeholder file with empty values instead of the repo's
# file (which contains a real-looking JWT secret). Real values are injected
# at runtime through env vars: PORT, DB_URL, JWT_SECRET_KEY.
RUN mkdir -p envs \
 && printf 'PORT=:50051\nDB_URL=\nJWT_SECRET_KEY=\n' > envs/dev.env

USER app

# 8082: HTTP (hardcoded in main.go) | 50051: gRPC (internal only, never publish it)
EXPOSE 8082 50051

ENTRYPOINT ["./auth"]
