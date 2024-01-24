#!/bin/bash

set -eu

mkdir -p ./tmp/pkg

WORKDIR=$(pwd)
PKG=e2fsprogs
MKFS_CONF="$SNAPCRAFT_PART_INSTALL"/etc/ubuntu-image/mkfs

mkdir -p "$MKFS_CONF"

ubuntu-distro-info --supported -f > supported
ubuntu-distro-info --supported-esm -f > supported_esm

TOTAL_SERIES=$(sort supported supported_esm | uniq)

IFS=$'\n'
for FULL_SERIES in $TOTAL_SERIES; do
    SERIES_RELEASE=$(echo "$FULL_SERIES" | awk '{split($0,r,"\""); print tolower(r[2])}' | cut -d " " -f 1)
    SERIES_CODENAME=$(echo "$FULL_SERIES" | cut -d " " -f 2)

    cd "$WORKDIR"/tmp/ && \
    pull-lp-debs -a "$SNAPCRAFT_TARGET_ARCH" -p debs -d "$PKG $SERIES_RELEASE"-updates > in_updates.txt || true
    IN_UPDATES=$(grep "Found" "$WORKDIR"/tmp/in_updates.txt || true)

    if [ -z "$IN_UPDATES" ]; then
        cd "$WORKDIR"/tmp/ && \
        pull-lp-debs -a "$SNAPCRAFT_TARGET_ARCH" -p debs -d $PKG "$SERIES_CODENAME"
    fi

    dpkg-deb --extract "$(find . "${WORKDIR}/tmp/" -name "e2fsprogs_*.deb" | tail -1)" "$WORKDIR"/tmp/pkg/
    mkdir -p "$MKFS_CONF"/"$SERIES_CODENAME"/
    ln -s -r "$MKFS_CONF"/"$SERIES_CODENAME"/ "$MKFS_CONF"/"$SERIES_RELEASE"
    cp "$WORKDIR"/tmp/pkg/etc/mke2fs.conf "$MKFS_CONF"/"$SERIES_CODENAME"/

    rm  -r "$WORKDIR"/tmp/*.deb "$WORKDIR"/tmp/pkg/* "$WORKDIR"/tmp/in_updates.txt
done
