module github.com/jonasbg/paste/cli

go 1.25.0

require (
	github.com/gorilla/websocket v1.5.3
	github.com/jonasbg/paste/crypto v0.0.0
)

require golang.org/x/crypto v0.45.0 // indirect

replace github.com/jonasbg/paste/crypto => ../crypto
