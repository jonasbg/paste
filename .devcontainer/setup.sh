#!/bin/bash
set -e

# Define Go version to match your devcontainer image (adjust if needed)
GO_VERSION=1.25

# Download wasm_exec.js for current Go version to /tmp
echo "Downloading wasm_exec.js for Go ${GO_VERSION}..."
wget -q -O /tmp/wasm_exec.js "https://raw.githubusercontent.com/golang/go/release-branch.go${GO_VERSION}/misc/wasm/wasm_exec.js"

# Setup Go WASM environment
echo "Setting up web/static directory..."
mkdir -p web/static
cp /tmp/wasm_exec.js web/static/

# Install frontend dependencies
echo "Installing frontend dependencies..."
cd web && npm install && cd ..

# Build Go WASM module
echo "Compiling Go WASM module..."
GOOS=js GOARCH=wasm go build -o web/static/encryption.wasm ./wasm/wasm.go

echo "Setup complete."
