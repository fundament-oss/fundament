FROM golang:1.26.6-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /demo-plugin .

FROM alpine:3.21
COPY --from=build /demo-plugin /usr/local/bin/demo-plugin
COPY definition.yaml /app/definition.yaml
WORKDIR /app
ENTRYPOINT ["demo-plugin"]
