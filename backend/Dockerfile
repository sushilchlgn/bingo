FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/bingo-server .

FROM alpine:3.20
RUN adduser -D -H bingo
WORKDIR /app
COPY --from=build /out/bingo-server /app/bingo-server
COPY static /app/static
USER bingo
EXPOSE 8080
ENV BINGO_ALLOWED_ORIGINS=""
CMD ["/app/bingo-server"]
