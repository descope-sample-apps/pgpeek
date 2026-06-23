# --- build stage ---------------------------------------------------------
FROM golang:1.26-alpine@sha256:3ad57304ad93bbec8548a0437ad9e06a455660655d9af011d58b993f6f615648 AS build
WORKDIR /src

# Cache deps first.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Pure-Go build (modernc.org/sqlite + pgx) → static binary, no cgo.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /pgpeek .

# --- runtime stage -------------------------------------------------------
# distroless: no shell, minimal attack surface; nonroot user.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:d093aa3e30dbadd3efe1310db061a14da60299baff8450a17fe0ccc514a16639
WORKDIR /
COPY --from=build /pgpeek /pgpeek

ENV PGPEEK_LISTEN=:8080 \
    PGPEEK_STORE_PATH=/data/pgpeek.db
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/pgpeek"]
