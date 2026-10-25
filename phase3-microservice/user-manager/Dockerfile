# ---------- Stage 1: build ----------
FROM golang:1.22-alpine AS build

# Same mirrors as the auth service (proxy.golang.org is blocked from Iran).
ARG GOPROXY=https://mirror.abrha.net/repository/go/|https://mirror-go.runflare.com|https://package-mirror.liara.ir/repository/go/|direct
ARG GOSUMDB=off

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/user-manager .

# ---------- Stage 2: runtime ----------
FROM alpine:3.20
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=build /out/user-manager ./user-manager
USER app
EXPOSE 8090
ENTRYPOINT ["./user-manager"]
