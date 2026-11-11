FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -o /out/core ./cmd/core
FROM alpine:3.20
RUN adduser -D -u 10001 reforge && mkdir -p /data && chown reforge /data
COPY --from=build /out/core /usr/local/bin/core
USER reforge
CMD ["core"]
