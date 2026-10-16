FROM oven/bun:1.3 AS web
WORKDIR /src/web
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY web/ ./
RUN bun run build

FROM golang:1.27-alpine AS server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server

FROM alpine:3.24
RUN apk add --no-cache git ca-certificates
COPY --from=server /out/server /usr/local/bin/server
ENV LISTEN_ADDR=:8080 REPOS_DIR=/data/repos
EXPOSE 8080
ENTRYPOINT ["server"]
