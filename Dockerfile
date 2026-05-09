# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src
COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -C cmd -trimpath -ldflags="-s -w" -o /out/valed .

FROM --platform=$BUILDPLATFORM alpine:3 AS optimize

RUN apk add --no-cache upx

COPY --from=build /out/valed /out/valed

RUN upx --best --lzma /out/valed

FROM alpine:3

RUN apk add --no-cache ca-certificates tzdata

COPY --from=optimize /out/valed /usr/local/bin/valed

USER 65532:65532
EXPOSE 8080 19090

ENTRYPOINT ["/usr/local/bin/valed"]
