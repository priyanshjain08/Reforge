FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -o /out/agent ./cmd/agent
FROM alpine:3.20
COPY --from=build /out/agent /usr/local/bin/agent
CMD ["agent"]
