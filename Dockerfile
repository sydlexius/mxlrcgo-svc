# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/mxlrcgo-svc ./cmd/mxlrcgo-svc

FROM alpine:3.22

LABEL org.opencontainers.image.source="https://github.com/sydlexius/mxlrcgo-svc" \
      org.opencontainers.image.description="Fetch synced lyrics from Musixmatch and save .lrc files" \
      org.opencontainers.image.licenses="MIT"

RUN apk add --no-cache ca-certificates su-exec tzdata && \
    { addgroup mxlrcgo 2>/dev/null || true; } && \
    { adduser -u 99 -G mxlrcgo -s /bin/sh -D mxlrcgo 2>/dev/null || true; } && \
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
