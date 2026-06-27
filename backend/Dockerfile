FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
ARG TARGET=api
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${TARGET}

FROM alpine:3.21
RUN apk add --no-cache ca-certificates ffmpeg
WORKDIR /app
COPY --from=build /out/app /usr/local/bin/app
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/app"]
