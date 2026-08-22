FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/videohub ./cmd/videohub

FROM alpine:3.20
RUN apk add --no-cache su-exec \
  && adduser -D -H -u 1000 videohub
WORKDIR /app
COPY --from=build /out/videohub /app/videohub
COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh
ENV VIDEOHUB_ADDR=:8080 \
    VIDEOHUB_VIDEO_DIR=/videos \
    PUID=1000 \
    PGID=1000
EXPOSE 8080
# Start as root so entrypoint can fix /videos ownership, then drop privileges.
ENTRYPOINT ["/docker-entrypoint.sh"]
