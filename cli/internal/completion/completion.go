package completion

import (
	"fmt"
)

// BashCompletion returns the bash completion script
func BashCompletion() string {
	return `# paste completion for bash

_paste_completion() {
    local cur prev opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    # Main commands
    local commands="upload download version help completion"

    # Flags for upload
    local upload_flags="-f -n -url"

    # Flags for download
    local download_flags="-l -o -url"

    # Complete main command if we're on word 1
    if [ $COMP_CWORD -eq 1 ]; then
        COMPREPLY=( $(compgen -W "${commands}" -- ${cur}) )
        return 0
    fi

    # Handle subcommands
    local cmd="${COMP_WORDS[1]}"

    case "${cmd}" in
        upload)
            case "${prev}" in
                -f)
                    # Complete files and directories
                    COMPREPLY=( $(compgen -f -- ${cur}) )
                    return 0
                    ;;
                -n|-url)
                    # No completion for these
                    return 0
                    ;;
                *)
                    COMPREPLY=( $(compgen -W "${upload_flags}" -- ${cur}) )
                    return 0
                    ;;
            esac
            ;;
        download)
            case "${prev}" in
                -o)
                    # Complete files
                    COMPREPLY=( $(compgen -f -- ${cur}) )
                    return 0
                    ;;
                -l|-url)
                    # No completion for these
                    return 0
                    ;;
                *)
                    COMPREPLY=( $(compgen -W "${download_flags}" -- ${cur}) )
                    return 0
                    ;;
            esac
            ;;
        completion)
            COMPREPLY=( $(compgen -W "bash zsh fish" -- ${cur}) )
            return 0
            ;;
    esac
}

complete -F _paste_completion paste
`
}

// ZshCompletion returns the zsh completion script
func ZshCompletion() string {
	return `#compdef paste

_paste() {
    local -a commands
    commands=(
        'upload:Upload a file or stdin'
        'download:Download a file'
        'version:Show version'
        'help:Show help'
        'completion:Generate shell completion'
    )

    local -a upload_args
    upload_args=(
        '-f[File to upload]:file:_files'
        '-n[Override filename]:filename:'
        '-url[Paste server URL]:url:'
    )

    local -a download_args
    download_args=(
        '-l[Download link]:link:'
        '-o[Output file]:file:_files'
        '-url[Paste server URL]:url:'
    )

    local -a completion_args
    completion_args=(
        'bash:Generate bash completion'
        'zsh:Generate zsh completion'
        'fish:Generate fish completion'
    )

    _arguments -C \
        '1: :->cmds' \
        '*:: :->args'

    case $state in
        cmds)
            _describe 'command' commands
            ;;
        args)
            case $line[1] in
                upload)
                    _arguments $upload_args
                    ;;
                download)
                    _arguments $download_args
                    ;;
                completion)
                    _describe 'shell' completion_args
                    ;;
            esac
            ;;
    esac
}

_paste
`
}

// FishCompletion returns the fish completion script
func FishCompletion() string {
	return `# paste completion for fish

# Main commands
complete -c paste -f -n __fish_use_subcommand -a upload -d 'Upload a file or stdin'
complete -c paste -f -n __fish_use_subcommand -a download -d 'Download a file'
complete -c paste -f -n __fish_use_subcommand -a version -d 'Show version'
complete -c paste -f -n __fish_use_subcommand -a help -d 'Show help'
complete -c paste -f -n __fish_use_subcommand -a completion -d 'Generate shell completion'

# Upload command
complete -c paste -n '__fish_seen_subcommand_from upload' -s f -l file -d 'File to upload' -r
complete -c paste -n '__fish_seen_subcommand_from upload' -s n -l name -d 'Override filename' -r
complete -c paste -n '__fish_seen_subcommand_from upload' -l url -d 'Paste server URL' -r

# Download command
complete -c paste -n '__fish_seen_subcommand_from download' -s l -l link -d 'Download link' -r
complete -c paste -n '__fish_seen_subcommand_from download' -s o -l output -d 'Output file' -r
complete -c paste -n '__fish_seen_subcommand_from download' -l url -d 'Paste server URL' -r

# Completion command
complete -c paste -n '__fish_seen_subcommand_from completion' -f -a 'bash zsh fish'
`
}

// PrintCompletion prints the completion script for the given shell
func PrintCompletion(shell string) error {
	switch shell {
	case "bash":
		fmt.Print(BashCompletion())
	case "zsh":
		fmt.Print(ZshCompletion())
	case "fish":
		fmt.Print(FishCompletion())
	default:
		return fmt.Errorf("unsupported shell: %s (supported: bash, zsh, fish)", shell)
	}
	return nil
}
