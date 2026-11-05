FROM golang:1.26.6-alpine AS build
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /app/server .
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /app/server /server
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/server"]
