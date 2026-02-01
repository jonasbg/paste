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
	uploadPassphrase := uploadCmd.Int("p", 5, "Number of words in passphrase (3-8, default: 5, use --url-mode for legacy URLs)")
	uploadPassphraseAlt := uploadCmd.Int("passphrase", 0, "Number of words in passphrase (3-8, default: 5, use --url-mode for legacy URLs)")
	uploadURLMode := uploadCmd.Bool("url-mode", false, "Use legacy URL mode instead of passphrase")

	sendFile := sendCmd.String("f", "", "File to send (omit to read from stdin)")
	sendName := sendCmd.String("n", "", "Override filename (default: uses file name or 'stdin.txt')")
	sendURL := sendCmd.String("url", a.pasteURL, "Paste server URL")
	sendPassphrase := sendCmd.Int("p", 5, "Number of words in passphrase (3-8, default: 5, use --url-mode for legacy URLs)")
	sendPassphraseAlt := sendCmd.Int("passphrase", 0, "Number of words in passphrase (3-8, default: 5, use --url-mode for legacy URLs)")
	sendURLMode := sendCmd.Bool("url-mode", false, "Use legacy URL mode instead of passphrase")

	// Download flags
	downloadLink := downloadCmd.String("l", "", "Download link (format: https://paste.torden.tech/{id}#key={key})")
	downloadOutput := downloadCmd.String("o", "", "Output file (default: original filename or stdout)")
	downloadURL := downloadCmd.String("url", a.pasteURL, "Paste server URL")

	// If no args provided
	if len(args) < 1 {
		if stdinIsPiped {
			// Default to upload from stdin with passphrase
			return a.handleUpload("", "", a.pasteURL, 5)
		}
		printUsage()
		return errors.New("no command provided")
	}

	// If first arg is a flag and stdin is piped, treat as upload
	if strings.HasPrefix(args[0], "-") && stdinIsPiped {
		uploadCmd.Parse(args)
		passphraseWords := *uploadPassphrase
		if *uploadPassphraseAlt > 0 {
			passphraseWords = *uploadPassphraseAlt
		}
		if *uploadURLMode {
			passphraseWords = 0 // Use URL mode
		}
		return a.handleUpload(*uploadFile, *uploadName, *uploadURL, passphraseWords)
	}

	switch args[0] {
	case "upload":
		uploadCmd.Parse(args[1:])
		passphraseWords := *uploadPassphrase
		if *uploadPassphraseAlt > 0 {
			passphraseWords = *uploadPassphraseAlt
		}
		if *uploadURLMode {
			passphraseWords = 0 // Use URL mode
		}
		return a.handleUpload(*uploadFile, *uploadName, *uploadURL, passphraseWords)

	case "send":
		sendCmd.Parse(args[1:])
		if *sendFile == "" {
			if extraArgs := sendCmd.Args(); len(extraArgs) > 0 {
				*sendFile = extraArgs[len(extraArgs)-1]
			}
		}
		passphraseWords := *sendPassphrase
		if *sendPassphraseAlt > 0 {
			passphraseWords = *sendPassphraseAlt
		}
		if *sendURLMode {
			passphraseWords = 0 // Use URL mode
		}
		return a.handleUpload(*sendFile, *sendName, *sendURL, passphraseWords)

	case "download":
		// Find passphrase/link in any position (non-flag argument)
		var foundLink string
		var filteredArgs []string
		for i := 1; i < len(args); i++ {
			arg := args[i]
			// Skip flags and their values
			if strings.HasPrefix(arg, "-") {
				filteredArgs = append(filteredArgs, arg)
				// If it's a flag that takes a value, include the next arg too
				if (arg == "-l" || arg == "-o" || arg == "--url") && i+1 < len(args) {
					i++
					filteredArgs = append(filteredArgs, args[i])
				}
			} else if foundLink == "" {
				// First non-flag argument is the passphrase/link
				foundLink = arg
			}
		}

		downloadCmd.Parse(filteredArgs)

		// Use found passphrase or fall back to -l flag
		if foundLink != "" {
			*downloadLink = foundLink
		}

		if *downloadLink == "" {
			fmt.Fprintf(os.Stderr, "Error: download link or passphrase is required\n")
			downloadCmd.PrintDefaults()
			return errors.New("download link or passphrase is required")
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

func (a *App) handleUpload(filePath, customName, serverURL string, passphraseWords int) error {
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

	// Create upload handler
	handler := upload.NewHandler(serverURL, config)

	// Check if passphrase mode is enabled
	if passphraseWords > 0 {
		// Validate word count
		if passphraseWords < 3 || passphraseWords > 8 {
			return fmt.Errorf("passphrase word count must be between 3 and 8, got %d", passphraseWords)
		}

		// Upload with passphrase
		passphrase, err := handler.UploadWithPassphrase(reader, filename, contentType, fileSize, passphraseWords)
		if err != nil {
			return err
		}

		// Print result
		fmt.Printf("\n✓ Upload complete!\n")
		fmt.Printf("\nShare code: %s\n", passphrase)
		fmt.Printf("\nDownload with: pastectl download %s\n", passphrase)
	} else {
		// Traditional URL-based mode
		key, err := crypto.GenerateKey(config.KeySize / 8)
		if err != nil {
			return fmt.Errorf("failed to generate key: %w", err)
		}

		shareURL, err := handler.Upload(reader, filename, contentType, fileSize, key)
		if err != nil {
			return err
		}

		// Print result
		fmt.Printf("\n%s\n", shareURL)
		fmt.Printf("\nDownload with: pastectl download -l \"%s\"\n", shareURL)
	}
	return nil
}

func (a *App) handleDownload(link, outputPath, serverURL string) error {
	// Check if input is a passphrase instead of a URL
	if download.IsPassphrase(link) {
		// Create client and get config
		c := client.New(serverURL)
		config, err := c.GetConfig()
		if err != nil {
			return fmt.Errorf("failed to get server config: %w", err)
		}

		// Create download handler and download with passphrase
		handler := download.NewHandler(c, config)
		return handler.DownloadWithPassphrase(link, outputPath)
	}

	// Traditional URL-based download
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
	fmt.Fprintf(os.Stderr, `pastectl v%s - Upload and download files with simple passphrases

Usage:
	pastectl [flags]               Upload from stdin (when piped/redirected)
	pastectl upload [flags]        Upload a file, directory, or stdin
	pastectl send [flags]          Send a file, directory, or stdin
	pastectl download <passphrase|url> [flags]  Download a file
	pastectl completion <shell>    Generate shell completion (bash, zsh, fish)
	pastectl version               Show version
	pastectl help                  Show this help

Upload Examples (Passphrase Mode - Default):
	echo "Hello World" | pastectl
	  → Share code: happy-ocean-mountain-forest-river-x7k3

	pastectl upload -f document.pdf
	  → Share code: calm-river-sunset-moon-peak-a2b9

	pastectl send presentation.pdf -p 3
	  → Share code: calm-river-sunset-f4m2 (3 words + suffix)

	pastectl upload -f file.txt --url-mode
	  → https://... (legacy URL mode)

Download Examples:
	pastectl download happy-ocean-forest-moon-river-x7k3
	pastectl download happy-ocean-forest-moon-river-x7k3 -o output.txt
	pastectl download -l "https://paste.torden.tech/abc123#key=xyz..."  # Legacy URLs still work

Upload Flags:
	-f <file>          File to upload (omit to read from stdin)
	-n <name>          Override filename
	-p <N>             Number of words in passphrase (3-8, default: 5)
	--passphrase <N>   Same as -p
	--url-mode         Use legacy URL mode instead of passphrase
	--url <url>        Custom server URL

Download Flags:
	-l <passphrase|url>  Passphrase or legacy URL (can also be positional)
	-o <file>            Output file (default: original filename or stdout)
	--url <url>          Custom server URL

Shell Completion:
	# Bash
	pastectl completion bash > /etc/bash_completion.d/pastectl

	# Zsh
	pastectl completion zsh > "${fpath[1]}/_pastectl"

	# Fish
	pastectl completion fish > ~/.config/fish/completions/pastectl.fish

Important Notes:
	- Default mode uses 5-word passphrases + random suffix (~60 bits entropy)
	- Format: word-word-word-word-word-x7k3 (words + 4-char suffix)
	- Fewer words: use -p 3 for easier sharing (less entropy)
	- More words: use -p 7 or -p 8 for sensitive files (more entropy)
	- Legacy URL mode still available with --url-mode flag
	- Directories are automatically compressed as tar.gz archives

Environment Variables:
	PASTE_URL    Default paste server URL (default: %s)

`, Version, DefaultURL)
}
