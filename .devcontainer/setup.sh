#!/bin/bash

# Setup Go WASM environment
echo "Setting up web/static directory..."
mkdir -p /workspaces/paste/web/static
cp /usr/local/go/lib/wasm/wasm_exec.js /workspaces/paste/web/static/

# Install frontend dependencies
echo "Installing frontend dependencies..."
cd /workspaces/paste/web && npm install && cd ..

# Build Go WASM module
echo "Compiling Go WASM module..."
cd /workspaces/paste/wasm
GOOS=js GOARCH=wasm go build -o /workspaces/paste/web/static/encryption.wasm wasm.go

echo "Setup complete."
