# Extends the maintained joedwards32/cs2 dedicated-server image. The orchestrator
# now fetches/extracts mods and writes core.json host-side, so the in-container
# hook (hooks/pre.sh) needs no unzip/jq — we only ensure ca-certificates for TLS.
# The base image runs as the unprivileged `steam` user, so we briefly switch to
# root to apt-install, then switch back so the original entrypoint behaves.
FROM joedwards32/cs2:latest

USER root
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
USER steam
