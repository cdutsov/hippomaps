# syntax=docker/dockerfile:1
FROM golang:1.27.3-alpine3.24 AS build
# compile with 2 cores: a build on a live server must not slow it down
ENV GOMAXPROCS=2
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go vet ./... && CGO_ENABLED=0 go test -count=1 ./...
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/hippomaps ./cmd/hippomaps

FROM alpine:3.24.3
RUN apk add --no-cache ca-certificates tzdata && adduser -D -H -u 10004 hippomaps
COPY --from=build /out/hippomaps /usr/local/bin/hippomaps
USER 10004:10004
EXPOSE 8096
HEALTHCHECK --interval=30s --timeout=5s --retries=3 --start-period=20s --start-interval=1s CMD ["hippomaps", "healthcheck"]
ENTRYPOINT ["hippomaps"]
CMD ["serve"]
