#!/bin/sh
set -e

PUID="${PUID:-99}"
PGID="${PGID:-100}"

if [ "$(id -u)" = "0" ]; then
    CURRENT_GID="$(id -g mxlrcgo 2>/dev/null || echo '')"
    CURRENT_UID="$(id -u mxlrcgo 2>/dev/null || echo '')"
    PGID_GROUP="$(awk -F: -v gid="${PGID}" '$3 == gid { print $1; exit }' /etc/group)"

    if [ "${CURRENT_GID}" != "${PGID}" ]; then
        deluser mxlrcgo 2>/dev/null || true
        if [ -z "${PGID_GROUP}" ]; then
            delgroup mxlrcgo 2>/dev/null || true
            addgroup -g "${PGID}" mxlrcgo
            PGID_GROUP="mxlrcgo"
        elif [ "${PGID_GROUP}" != "mxlrcgo" ]; then
            delgroup mxlrcgo 2>/dev/null || true
        fi
    fi

    if [ "${CURRENT_UID}" != "${PUID}" ] || [ "${CURRENT_GID}" != "${PGID}" ]; then
        deluser mxlrcgo 2>/dev/null || true
        adduser -u "${PUID}" -G "${PGID_GROUP:-mxlrcgo}" -s /bin/bash -D mxlrcgo
    fi

    chown -R mxlrcgo:"${PGID_GROUP:-mxlrcgo}" /config /music 2>/dev/null || true
    exec su-exec mxlrcgo "$@"
fi

exec "$@"
