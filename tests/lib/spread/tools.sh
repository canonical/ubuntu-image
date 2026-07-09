tools.setup_snapd_proxy() {
  if [ "${SNAPD_USE_PROXY:-}" != true ]; then
    return
  fi

  local SNAPD_CONFD="/etc/systemd/system/snapd.service.d"
  mkdir -p "$SNAPD_CONFD"

  cat <<EOF > ${SNAPD_CONFD}/proxy.conf
[Service]
Environment=HTTPS_PROXY="$HTTPS_PROXY" HTTP_PROXY="$HTTP_PROXY" https_proxy="$HTTPS_PROXY" http_proxy="$HTTP_PROXY" NO_PROXY="$NO_PROXY" no_proxy="$NO_PROXY"
EOF

  # Since the service config changed, restart
  systemctl daemon-reload
  systemctl restart snapd.service
}

tools.setup_system_proxy() {
    mkdir -p "$SNAPD_WORK_DIR"
    if [ "${SNAPD_USE_PROXY:-}" = true ]; then    
        cp -f /etc/environment "$SNAPD_WORK_DIR"/environment.bak
        {
            echo "HTTPS_PROXY=$HTTPS_PROXY"
            echo "HTTP_PROXY=$HTTP_PROXY"
            echo "https_proxy=$HTTPS_PROXY"
            echo "http_proxy=$HTTP_PROXY"
            echo "NO_PROXY=$NO_PROXY"
            echo "no_proxy=$NO_PROXY"
        } >> /etc/environment
    fi
}