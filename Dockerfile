# syntax=docker/dockerfile:1
FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/trimproof-gateway ./cmd/gateway \
 && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/trimproof ./cmd/trimproof \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/trimproof-gateway /out/trimproof /usr/local/bin/
COPY examples/quickstart.json /etc/trimproof/policies.json
# Runtime JSONL (audit, eval pairs, promotion transitions) is written to /data;
# mount a volume there to keep promotion state across restarts.
COPY --from=build --chown=nonroot:nonroot /out/data /data
WORKDIR /data
USER nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/trimproof-gateway", "-policies", "/etc/trimproof/policies.json"]
