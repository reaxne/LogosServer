FROM golang:1.25-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /server .

FROM alpine:3.22
RUN adduser -D -H appuser
USER appuser
COPY --from=build /server /server
EXPOSE 8080
ENV GIN_MODE=release
CMD ["/server"]
