FROM golang:1.23-alpine AS wasm-builder
WORKDIR /app
COPY . .
RUN GOOS=js GOARCH=wasm go build -o encryption.wasm wasm.go
RUN cp "$(go env GOROOT)/misc/wasm/wasm_exec.js" .

FROM golang:1.23-alpine AS backend-builder
RUN apk add --update gcc musl-dev --no-cache
WORKDIR /app
COPY go.mod *.sum ./
RUN go mod download && go mod verify
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -a \
    -ldflags='-w -s -linkmode external -extldflags "-static"' \
    -o droply main.go

FROM scratch
ENV GIN_MODE=release
COPY --from=backend-builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=backend-builder --chown=101:101 /app/droply /droply
COPY --from=wasm-builder --chown=101:101 /app/encryption.wasm /encryption.wasm
COPY --from=wasm-builder --chown=101:101 /app/wasm_exec.js /wasm_exec.js
COPY --chown=101:101 index.html /index.html
VOLUME ["/upload"]
USER 101
EXPOSE 8080
ENTRYPOINT ["/droply"]
CMD []