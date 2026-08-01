FROM node:26-alpine@sha256:725aeba2364a9b16beae49e180d83bd597dbd0b15c47f1f28875c290bfd255b9 AS web-vendor
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci --ignore-scripts
COPY web/vendor/src ./web/vendor/src
RUN npm run vendor

FROM ghcr.io/verity-org/golang:1.26-fips@sha256:c1689432748fd0cbeccec073c1d0d0a9a46acbfdc51e1ffaa16ea408871dca02 AS build
ARG VERSION=0.0.0-dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
SHELL ["/usr/bin/bash", "-c"]
ENV GOFIPS140=v1.0.0 \
    GODEBUG=fips140=on
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web-vendor /src/web/vendor ./web/vendor
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_DATE}" -o /src/pgpeek .
RUN printf '' > /src/data.keep

# --- runtime stage -------------------------------------------------------
# distroless: no shell, minimal attack surface; nonroot user.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:d093aa3e30dbadd3efe1310db061a14da60299baff8450a17fe0ccc514a16639
WORKDIR /
COPY --from=build /src/pgpeek /pgpeek
COPY --from=build --chown=nonroot:nonroot /src/data.keep /data/.keep

ENV PGPEEK_LISTEN=:8080 \
    PGPEEK_STORE_PATH=/data/pgpeek.db \
    GODEBUG=fips140=on \
    GOFIPS140=v1.0.0
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/pgpeek"]
