module github.com/jonasbg/paste/pastectl

go 1.26.0

require (
	github.com/gorilla/websocket v1.5.3
	github.com/jonasbg/paste/crypto v0.0.0
)

require (
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
)

replace github.com/jonasbg/paste/crypto => ../crypto
