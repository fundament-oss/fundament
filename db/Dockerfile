FROM ghcr.io/printeers/trek:0.12.6
COPY --chown=unprivileged:unprivileged ./trek.yaml /data
COPY --chown=unprivileged:unprivileged ./testdata /data/testdata
COPY --chown=unprivileged:unprivileged ./migrations /data/migrations
COPY --chown=unprivileged:unprivileged ./hooks /data/hooks
COPY --chown=unprivileged:unprivileged ./seed /data/seed
