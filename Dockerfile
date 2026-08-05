FROM golang:1.26.5-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /tripmate-api ./adapters/rest

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /tripmate-api /tripmate-api
EXPOSE 8080
ENTRYPOINT ["/tripmate-api"]

