# The web app's image: the page is built, then the Go server, then an
# Alpine runtime with ffmpeg and yt-dlp in it. The desktop app stays out;
# the plugins ship as they are.

FROM node:20-alpine AS front
WORKDIR /app/front
COPY front/package.json front/package-lock.json* ./
RUN npm ci --no-audit --no-fund || npm install --no-audit --no-fund
COPY front/ ./
RUN npm run build

FROM golang:1.27-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
ENV GOPROXY=https://goproxy.cn,direct
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY --from=front /app/front/dist ./cmd/web/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /out/codie-music-web ./cmd/web

FROM alpine:3.21
ARG VERSION=dev
RUN apk add --no-cache ca-certificates tzdata ffmpeg yt-dlp curl
WORKDIR /app
COPY --from=build /out/codie-music-web ./codie-music-web
COPY plugins ./plugins
RUN mkdir -p /app/data
VOLUME /app/data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s \
  CMD curl -sf http://127.0.0.1:8080/api/health || exit 1
ENV VERSION=${VERSION}
ENTRYPOINT ["./codie-music-web"]
CMD ["-addr", ":8080", "-data", "/app/data", "-plugins", "/app/plugins"]
