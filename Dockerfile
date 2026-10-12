# syntax=docker/dockerfile:1

FROM docker.io/library/golang:1.26.8 AS build
WORKDIR /src
ARG VERSION=dev
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY robothub ./robothub
COPY robot ./robot
COPY screen ./screen
COPY sound ./sound
COPY leds ./leds
COPY gesture ./gesture
COPY client ./client
COPY examples ./examples
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/q41eu-dev ./cmd/q41eu-dev

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/q41eu-dev /q41eu-dev
EXPOSE 8785
USER 65532:65532
# Mount the robot token at /secrets/robot-token and pass -public-url https://<host>; keep the state
# (pairings, API keys) in Postgres (-state-database or STATE_DATABASE_URL) or on a volume (-state-file).
ENTRYPOINT ["/q41eu-dev", "-listen", ":8785", "-token-file", "/secrets/robot-token", "-state-file", ""]
