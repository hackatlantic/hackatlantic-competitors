FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/server \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/seed-intake ./cmd/seed-intake \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/bootstrap-organizer ./cmd/bootstrap-organizer
RUN mkdir -p /out/resumes && chown 65532:65532 /out/resumes

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/api /api
COPY --from=build /out/migrate /migrate
COPY --from=build /out/seed-intake /seed-intake
COPY --from=build /out/bootstrap-organizer /bootstrap-organizer
COPY --chown=nonroot:nonroot --from=build /out/resumes /data/resumes
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/api"]
