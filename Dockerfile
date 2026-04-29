# syntax=docker/dockerfile:1

FROM golang:1.26.2-alpine@sha256:c2a1f7b2095d046ae14b286b18413a05bb82c9bca9b25fe7ff5efef0f0826166 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/mxlrcgo-svc ./cmd/mxlrcgo-svc

FROM alpine:3.23.4@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11

LABEL org.opencontainers.image.source="https://github.com/sydlexius/mxlrcgo-svc" \
      org.opencontainers.image.description="Fetch synced lyrics from Musixmatch and save .lrc files" \
      org.opencontainers.image.licenses="MIT"

RUN apk add --no-cache ca-certificates su-exec tzdata && \
    { grep -q "^mxlrcgo:" /etc/group || addgroup mxlrcgo; } && \
    { id -u mxlrcgo >/dev/null 2>&1 || adduser -u 99 -G mxlrcgo -s /bin/sh -D mxlrcgo; } && \
    mkdir -p /config /music && \
    chown mxlrcgo:mxlrcgo /config /music

COPY --from=build /out/mxlrcgo-svc /usr/local/bin/mxlrcgo-svc
COPY build/docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENV MXLRC_DOCKER=true \
    MXLRC_SERVER_ADDR=0.0.0.0:50705

WORKDIR /config
EXPOSE 50705
VOLUME ["/config", "/music"]

# USER is intentionally omitted so entrypoint.sh can perform PUID/PGID
# remapping and volume ownership fixes as root before dropping to mxlrcgo.
ENTRYPOINT ["/entrypoint.sh"]
CMD ["mxlrcgo-svc", "serve"]
