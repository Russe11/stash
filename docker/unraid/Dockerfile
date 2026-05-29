# Fork build image for Unraid / self-hosting.
#
# This is the project's own proven multi-stage build
# (docker/build/x86_64/Dockerfile) with default build-args so it builds with a
# plain `docker build` — no `make docker-build` wrapper required.
#
# Build context MUST be the repository root (it copies ./ui, ./pkg, ./internal, …):
#
#     docker build -f docker/unraid/Dockerfile -t stash-ng:latest .
#
# Whatever branch is checked out in the build context is what gets compiled, so
# check out the branch you want to deploy (see docker/unraid/README.md) first.
# The resulting binary reports schema v86 and will migrate a v85 library forward
# (one-way — keep a backup; see the README).

# ---- Build Frontend ----
FROM node:24-alpine AS frontend
RUN apk add --no-cache make git
## cache node_modules separately
COPY ./ui/v2.5/package.json ./ui/v2.5/pnpm-lock.yaml /stash/ui/v2.5/
WORKDIR /stash
COPY Makefile /stash/
COPY ./graphql /stash/graphql/
COPY ./ui /stash/ui/
RUN npm install -g pnpm
RUN make pre-ui
RUN make generate-ui
ARG GITHASH=fork
ARG STASH_VERSION=fork-dev
# Build the UI directly with vite. NOT `make ui-only`: its make-prerequisites
# (ui-only -> generate -> generate-backend, and -> generate-login-locale) both run
# `go generate`, and Go is not present in this Node-only stage. The UI's own codegen
# (gqlgen) already ran via `make generate-ui` above, so `vite build` is all that's left.
RUN cd ui/v2.5 && \
    VITE_APP_DATE="$(date +'%Y-%m-%d %H:%M:%S')" \
    VITE_APP_GITHASH="$GITHASH" \
    VITE_APP_STASH_VERSION="$STASH_VERSION" \
    npm run build

# ---- Build Backend ----
FROM golang:1.25.9-alpine AS backend
RUN apk add --no-cache make alpine-sdk
WORKDIR /stash
COPY ./go* ./*.go Makefile gqlgen.yml .gqlgenc.yml /stash/
COPY ./graphql /stash/graphql/
COPY ./scripts /stash/scripts/
COPY ./pkg /stash/pkg/
COPY ./cmd /stash/cmd/
COPY ./internal /stash/internal/
# needed for generate-login-locale
COPY ./ui /stash/ui/
RUN make generate-backend generate-login-locale
COPY --from=frontend /stash /stash/
ARG GITHASH=fork
ARG STASH_VERSION=fork-dev
RUN make flags-release flags-pie stash

# ---- Final Runnable Image ----
FROM alpine:latest
LABEL org.opencontainers.image.title="stash-ng" \
      org.opencontainers.image.description="Forked Stash server (schema v86: deletion feed, moveFolder, folder stats, webhooks)"
RUN apk add --no-cache ca-certificates vips-tools ffmpeg
COPY --from=backend /stash/stash /usr/bin/
ENV STASH_CONFIG_FILE=/root/.stash/config.yml
EXPOSE 9999
ENTRYPOINT ["stash"]
