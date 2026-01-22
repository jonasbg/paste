package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jonasbg/paste/crypto"
	"github.com/jonasbg/paste/pastectl/internal/client"
	"github.com/jonasbg/paste/pastectl/internal/completion"
	"github.com/jonasbg/paste/pastectl/internal/download"
	"github.com/jonasbg/paste/pastectl/internal/upload"
)

const (
	DefaultURL = "https://paste.torden.tech"
)

// Version is set at build time via ldflags
var Version = "dev"

// App represents the CLI application
type App struct {
	pasteURL string
}

// New creates a new CLI app
func New() *App {
	// URL can be set via environment variable
	pasteURL := DefaultURL
	if envURL := os.Getenv("PASTE_URL"); envURL != "" {
		pasteURL = envURL
	}

	return &App{
		pasteURL: pasteURL,
	}
}

// Run runs the CLI application
func (a *App) Run(args []string) error {
	// Check if stdin is piped or redirected
	stat, _ := os.Stdin.Stat()
	stdinIsPiped := (stat.Mode() & os.ModeCharDevice) == 0

	uploadCmd := flag.NewFlagSet("upload", flag.ExitOnError)
	downloadCmd := flag.NewFlagSet("download", flag.ExitOnError)
	sendCmd := flag.NewFlagSet("send", flag.ExitOnError)

	// Upload flags
	uploadFile := uploadCmd.String("f", "", "File to upload (omit to read from stdin)")
	uploadName := uploadCmd.String("n", "", "Override filename (default: uses file name or 'stdin.txt')")
	uploadURL := uploadCmd.String("url", a.pasteURL, "Paste server URL")
	sendFile := sendCmd.String("f", "", "File to send (omit to read from stdin)")
	sendName := sendCmd.String("n", "", "Override filename (default: uses file name or 'stdin.txt')")
	sendURL := sendCmd.String("url", a.pasteURL, "Paste server URL")

	// Download flags
	downloadLink := downloadCmd.String("l", "", "Download link (format: https://paste.torden.tech/{id}#key={key})")
	downloadOutput := downloadCmd.String("o", "", "Output file (default: original filename or stdout)")
	downloadURL := downloadCmd.String("url", a.pasteURL, "Paste server URL")

	// If no args provided
	if len(args) < 1 {
		if stdinIsPiped {
			// Default to upload from stdin
			return a.handleUpload("", "", a.pasteURL)
		}
		printUsage()
		return errors.New("no command provided")
	}

	// If first arg is a flag and stdin is piped, treat as upload
	if strings.HasPrefix(args[0], "-") && stdinIsPiped {
		uploadCmd.Parse(args)
		return a.handleUpload(*uploadFile, *uploadName, *uploadURL)
	}

	switch args[0] {
	case "upload":
		uploadCmd.Parse(args[1:])
		return a.handleUpload(*uploadFile, *uploadName, *uploadURL)

	case "send":
		sendCmd.Parse(args[1:])
		if *sendFile == "" {
			if extraArgs := sendCmd.Args(); len(extraArgs) > 0 {
				*sendFile = extraArgs[len(extraArgs)-1]
			}
		}
		return a.handleUpload(*sendFile, *sendName, *sendURL)

	case "download":
		downloadCmd.Parse(args[1:])
		if *downloadLink == "" {
			fmt.Fprintf(os.Stderr, "Error: download link is required\n")
			downloadCmd.PrintDefaults()
			return errors.New("download link is required")
		}
		return a.handleDownload(*downloadLink, *downloadOutput, *downloadURL)

	case "version", "-v", "--version":
		fmt.Printf("pastectl v%s\n", Version)
		return nil

	case "help", "-h", "--help":
		printUsage()
		return nil

	case "completion":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: shell type required (bash, zsh, or fish)\n")
			fmt.Fprintf(os.Stderr, "Usage: pastectl completion <shell>\n")
			return errors.New("shell type required")
		}
		return completion.PrintCompletion(args[1])

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", args[0])
		printUsage()
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func (a *App) handleUpload(filePath, customName, serverURL string) error {
	// Prepare input
	reader, filename, contentType, fileSize, err := upload.PrepareInput(filePath, customName)
	if err != nil {
		return err
	}

	// Create client and get config
	c := client.New(serverURL)
	config, err := c.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get server config: %w", err)
	}

	if fileSize > config.MaxFileSizeBytes {
		return fmt.Errorf("file size (%d bytes) exceeds server limit (%d bytes)", fileSize, config.MaxFileSizeBytes)
	}

	// Generate encryption key
	key, err := crypto.GenerateKey(config.KeySize / 8)
	if err != nil {
		return fmt.Errorf("failed to generate key: %w", err)
	}

	// Create upload handler and upload
	handler := upload.NewHandler(serverURL, config)
	shareURL, err := handler.Upload(reader, filename, contentType, fileSize, key)
	if err != nil {
		return err
	}

	// Print result
	fmt.Printf("\n%s\n", shareURL)
	fmt.Printf("\nDownload with: pastectl download -l \"%s\"\n", shareURL)

	return nil
}

func (a *App) handleDownload(link, outputPath, serverURL string) error {
	// Parse the download link
	fileID, key, linkServerURL, err := download.ParseLink(link)
	if err != nil {
		return err
	}

	// Use server URL from link if present
	if linkServerURL != "" {
		serverURL = linkServerURL
	}

	// Create client and get config
	c := client.New(serverURL)
	config, err := c.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get server config: %w", err)
	}

	// Create download handler and download
	handler := download.NewHandler(c, config)
	return handler.Download(fileID, key, outputPath)
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `pastectl v%s - Upload and download files to paste.torden.tech

Usage:
	pastectl [flags]               Upload from stdin (when piped/redirected)
	pastectl upload [flags]        Upload a file, directory, or stdin
	pastectl send [flags]          Send a file, directory, or stdin
	pastectl download [flags]      Download a file
	pastectl completion <shell>    Generate shell completion (bash, zsh, fish)
	pastectl version               Show version
	pastectl help                  Show this help

Upload Examples:
	echo "Hello World" | pastectl
	cat file.txt | pastectl
	pastectl < myfile.txt
	echo "data" | pastectl -n "custom-name.txt"
	pastectl upload -f document.pdf
	pastectl upload -f my-directory/          # Uploads as tar.gz archive
	cat image.png | pastectl upload -n "my-image.png"
	pastectl upload -f file.txt -url https://custom.paste.server
	pastectl send file.txt

Download Examples:
	pastectl download -l "https://paste.torden.tech/abc123#key=xyz..."
	pastectl download -l "https://paste.torden.tech/abc123#key=xyz..." -o output.txt
	pastectl download -l "URL" -o archive.tar.gz  # Download directory archive

Shell Completion:
	# Bash
	pastectl completion bash > /etc/bash_completion.d/pastectl
	# Or for current user:
	pastectl completion bash >> ~/.bashrc

	# Zsh
	pastectl completion zsh > "${fpath[1]}/_pastectl"

	# Fish
	pastectl completion fish > ~/.config/fish/completions/pastectl.fish

Important Notes:
	- When using stdin (< file or |), the original filename is lost
	- Use -n flag to specify a custom filename for stdin uploads
	- Or use -f flag to preserve the original filename: pastectl upload -f file.mp4
	- Directories are automatically compressed as tar.gz archives
	- Content type is auto-detected from file data when possible

Environment Variables:
	PASTE_URL    Default paste server URL (default: %s)

`, Version, DefaultURL)
}
