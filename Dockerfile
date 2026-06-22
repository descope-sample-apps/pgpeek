# --- build stage ---------------------------------------------------------
FROM golang:1.26-alpine AS build
WORKDIR /src

# Cache deps first.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Pure-Go build (modernc.org/sqlite + pgx) → static binary, no cgo.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /pgpeek .

# --- runtime stage -------------------------------------------------------
# distroless: no shell, minimal attack surface; nonroot user.
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=build /pgpeek /pgpeek

ENV PGPEEK_LISTEN=:8080 \
    PGPEEK_STORE_PATH=/data/pgpeek.db
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/pgpeek"]
