#!/bin/bash

set -euo pipefail

WORKDIR=$(pwd)
TMP_DIR="$WORKDIR"/tmp/
PKG=e2fsprogs
MKFS_CONF="$WORKDIR"/mkfs/confs
DB="$WORKDIR"/mkfs/db

rm -r "${MKFS_CONF:?}"/* || true

if [ -f "$DB" ]; then
    rm "$DB" 
fi

mkdir -p "$MKFS_CONF"
mkdir -p "$TMP_DIR"/pkg

ubuntu-distro-info --supported -f > supported
ubuntu-distro-info --supported-esm -f > supported_esm

TOTAL_SERIES=$(sort supported supported_esm | uniq)

rm supported supported_esm

IFS=$'\n'
for FULL_SERIES in $TOTAL_SERIES; do
    SERIES_RELEASE=$(echo "$FULL_SERIES" | awk '{split($0,r,"\""); print tolower(r[2])}' | cut -d " " -f 1)
    SERIES_CODENAME=$(echo "$FULL_SERIES" | cut -d " " -f 2)
    SERIES_RELEASE_POCKET="$SERIES_RELEASE"-updates

    # Collect configurations from amd64 packages for now. This is working under the assumption the configuration
    # is the same for every arch. This may be wrong now or in the future.
    cd "$TMP_DIR" && \
    pull-lp-debs -a amd64 -p debs -d "$PKG $SERIES_RELEASE"-updates > in_updates.txt || true
    IN_UPDATES=$(grep "Found" "$TMP_DIR"/in_updates.txt || true)

    if [ -z "$IN_UPDATES" ]; then
        cd "$TMP_DIR" && \
        pull-lp-debs -a amd64 -p debs -d $PKG "$SERIES_CODENAME"
        SERIES_RELEASE_POCKET="$SERIES_RELEASE"
    fi

    PKG_NAME=$(find . "${TMP_DIR}" -name "${PKG}_*.deb" | tail -1)
    VERSION=$(echo "$PKG_NAME" | cut -d "_" -f 2)

    dpkg-deb --extract "$(find . "${TMP_DIR}" -name "${PKG}_*.deb" | tail -1)" "$TMP_DIR"/pkg/
    mkdir -p "$MKFS_CONF"/"$SERIES_CODENAME"/
    ln -s -r "$MKFS_CONF"/"$SERIES_CODENAME"/ "$MKFS_CONF"/"$SERIES_RELEASE"
    cp "$TMP_DIR"/pkg/etc/mke2fs.conf "$MKFS_CONF"/"$SERIES_CODENAME"/
    echo "$SERIES_RELEASE_POCKET,$VERSION" >> "$DB"

    rm  -r "$TMP_DIR"/*.deb "$TMP_DIR"/pkg/* "$TMP_DIR"/in_updates.txt
done

rm -r "$TMP_DIR"
