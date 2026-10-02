# syntax=docker/dockerfile:1

# ---- Stage 1: build web static ----
FROM node:20-alpine AS webbuild
WORKDIR /web
COPY web/package.json web/pnpm-lock.yaml ./
RUN corepack enable && corepack prepare pnpm@10.32.1 --activate && pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

# ---- Stage 2: build api binary (CGO not needed) ----
# 2 binary, 1 image (xem Stage 4 — app và audio dùng chung image, chỉ khác
# entrypoint):
#   /out/langapp-server  app (Gin + GraphQL + Postgres), entrypoint M4
#   /out/audio-service   tiến trình audio riêng (Piper exec + forward Whisper)
#
# STACK-V2 M7c: binary `/out/langapp` (app v1, net/http + SQLite) đã bị gỡ cùng
# mã nguồn ở `api/*.go`. `api/` giờ chỉ còn `cmd/`, `internal/`, `services/`…
FROM golang:1.27-alpine AS apibuild
WORKDIR /src
COPY api/go.mod api/go.sum ./api/
WORKDIR /src/api
RUN go mod download
COPY api/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/langapp-server ./cmd/langapp \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/audio-service ./services/audio-service

# ---- Stage 3: fetch Piper voices (T7.1, pinned files from rhasspy/piper-voices) ----
# Pinned HF commit c10ece1aade47bb51c153c893d14e5bf8e5b7117 (thay resolve/main):
# build lặp lại dùng đúng 2 file voice đã duyệt, không trôi theo main.
FROM debian:bookworm-slim AS voices
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*
ARG PIPER_VOICES_COMMIT=c10ece1aade47bb51c153c893d14e5bf8e5b7117
RUN mkdir -p /models/zh /models/en \
    && curl -sSL -o /models/zh/zh_CN-huayan-medium.onnx \
        https://huggingface.co/rhasspy/piper-voices/resolve/${PIPER_VOICES_COMMIT}/zh/zh_CN/huayan/medium/zh_CN-huayan-medium.onnx \
    && curl -sSL -o /models/zh/zh_CN-huayan-medium.onnx.json \
        https://huggingface.co/rhasspy/piper-voices/resolve/${PIPER_VOICES_COMMIT}/zh/zh_CN/huayan/medium/zh_CN-huayan-medium.onnx.json \
    && curl -sSL -o /models/en/en_US-lessac-medium.onnx \
        https://huggingface.co/rhasspy/piper-voices/resolve/${PIPER_VOICES_COMMIT}/en/en_US/lessac/medium/en_US-lessac-medium.onnx \
    && curl -sSL -o /models/en/en_US-lessac-medium.onnx.json \
        https://huggingface.co/rhasspy/piper-voices/resolve/${PIPER_VOICES_COMMIT}/en/en_US/lessac/medium/en_US-lessac-medium.onnx.json \
    && ls -la /models/zh /models/en

# ---- Stage 4: runtime — 1 image serve cả dist + API + Piper TTS (T7.1) ----
# Debian glibc cần thiết cho wheel piper1-gpl (manylinux, không chạy trên alpine musl).
FROM python:3.11-slim-bookworm
# M7c: bỏ `mkdir /data` — thư mục đó chỉ phục vụ file SQLite của app v1, đã gỡ.
RUN adduser --disabled-password --gecos "" --uid 10001 app \
    && mkdir -p /app/dist /models && chown app:app /app/dist
# Piper engine OHF-Voice/piper1-gpl v1.6.1 (GPL-3.0 — xem /app/THIRD-PARTY-LICENSES).
RUN pip install --no-cache-dir \
        https://github.com/OHF-Voice/piper1-gpl/releases/download/v1.6.1/piper_tts-1.6.1-cp39-abi3-manylinux_2_17_x86_64.manylinux2014_x86_64.manylinux_2_28_x86_64.whl \
    && piper --help >/dev/null
COPY --from=voices /models/ /models/
COPY --from=webbuild /web/dist/ /app/dist/
COPY --from=apibuild /out/langapp-server /app/langapp-server
# STACK-V2 M4: 2 binary cùng image, chỉ khác `entrypoint` (xem
# `docker-compose.yml`): `app-v2` chạy `/app/langapp-server`, `audio-service`
# chạy `/app/audio-service`. Tách image riêng cho audio sẽ tải 2 lần Piper +
# 2 voice 752MB cho cùng 2 file.
COPY --from=apibuild /out/audio-service /app/audio-service
COPY THIRD-PARTY-LICENSES /app/THIRD-PARTY-LICENSES
USER app
ENV PORT=8080 \
    WEB_DIST=/app/dist \
    PIPER_BIN=/usr/local/bin/piper \
    PIPER_MODEL_ZH=/models/zh/zh_CN-huayan-medium.onnx \
    PIPER_MODEL_EN=/models/en/en_US-lessac-medium.onnx \
    # M4: app đọc DSN Postgres qua `LANGAPP_POSTGRES_DSN`. `AUDIO_GRPC_ADDR`
    # rỗng = app tự dùng stub, nên không bật audio-service vẫn chạy được.
    AUDIO_GRPC_ADDR=
# M7c: bỏ `VOLUME ["/data"]` — chỉ phục vụ SQLite của app v1. Dữ liệu v2 nằm ở
# volume `postgres-data`, khai trong `docker-compose.yml`.
# 9090 = cổng gRPC của audio-service.
EXPOSE 8080 9090
# HEALTHCHECK chấp nhận CẢ `ok` LẪN `degraded`.
#
# `degraded` chỉ xuất hiện khi audio là stub, tức `AUDIO_GRPC_ADDR` rỗng — đó là
# cấu hình được hỗ trợ chính thức (mặc định ở dòng `AUDIO_GRPC_ADDR=` phía trên),
# nên coi nó là chết là sai: `app-v2` unhealthy vĩnh viễn, và M7 thêm service nào
# `depends_on: service_healthy` là treo. Postgres chết vẫn trả 503 ⇒ healthcheck
# vẫn bắt được thứ duy nhất thực sự chết người.
#
# Lý do KHÔNG đổi `/api/health` trả `ok` cho stub: `status` là thông cho người
# vận hành, còn healthcheck là câu hỏi "app còn phục vụ được không". Hai câu hỏi
# khác nhau nên hai chỗ khác nhau.
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD python3 -c "import urllib.request,sys; b=urllib.request.urlopen('http://127.0.0.1:8080/api/health', timeout=5).read(); sys.exit(0 if (b'\"status\":\"ok\"' in b or b'\"status\":\"degraded\"' in b) else 1)"
WORKDIR /app
ENTRYPOINT ["/app/langapp-server"]
