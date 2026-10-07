FROM node:22-alpine AS web
WORKDIR /web
COPY notes-web/package*.json ./
RUN npm ci
COPY notes-web/ ./
RUN npm run build

FROM web AS web-test
RUN npm test

FROM golang:1.26-bookworm AS server
WORKDIR /src
COPY notes-server-go/go.mod notes-server-go/go.sum ./
RUN go mod download
COPY notes-server-go/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /shiji ./cmd/shiji

FROM server AS server-test
RUN go test -race ./... && go vet ./...

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl \
    && mkdir -p /usr/share/postgresql-common/pgdg \
    && curl --fail --silent --show-error https://www.postgresql.org/media/keys/ACCC4CF8.asc -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc \
    && echo 'deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] https://apt.postgresql.org/pub/repos/apt bookworm-pgdg main' > /etc/apt/sources.list.d/pgdg.list \
    && apt-get update && apt-get install -y --no-install-recommends postgresql-client-16 \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --uid 10001 --create-home notes
WORKDIR /app
COPY --from=server /shiji /usr/local/bin/shiji
COPY --from=web /web/dist /app/web
RUN mkdir -p /data/attachments /data/exports && chown -R notes:notes /data /app
USER notes
ENV WEB_DIR=/app/web ATTACHMENTS_DIR=/data/attachments EXPORTS_DIR=/data/exports LISTEN_ADDR=0.0.0.0:8000
EXPOSE 8000
CMD ["sh", "-c", "shiji migrate && exec shiji serve"]
