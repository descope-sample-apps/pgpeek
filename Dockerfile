# --- build stage ---------------------------------------------------------
FROM golang:1.26-alpine@sha256:3ad57304ad93bbec8548a0437ad9e06a455660655d9af011d58b993f6f615648 AS build
WORKDIR /src
RUN apk add --no-cache nodejs npm

# Cache deps first.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN npm ci --ignore-scripts && npm run vendor
# Pure-Go build (modernc.org/sqlite + pgx) → static binary, no cgo.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /pgpeek .
RUN mkdir /data

# --- runtime stage -------------------------------------------------------
# distroless: no shell, minimal attack surface; nonroot user.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:d093aa3e30dbadd3efe1310db061a14da60299baff8450a17fe0ccc514a16639
WORKDIR /
COPY --from=build /pgpeek /pgpeek
COPY --from=build --chown=nonroot:nonroot /data /data

ENV PGPEEK_LISTEN=:8080 \
    PGPEEK_STORE_PATH=/data/pgpeek.db
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/pgpeek"]
