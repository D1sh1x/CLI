FROM golang:1.24-alpine AS build
WORKDIR /src
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/mygrep .

FROM scratch
COPY --from=build /out/mygrep /mygrep
WORKDIR /data
ENTRYPOINT ["/mygrep"]
CMD ["help"]
