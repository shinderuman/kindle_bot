#!/bin/bash

# Bash completion for deploy.sh script
_deploy_completion() {
    local cur prev opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    
    # Available function names
    local functions="paper-to-kindle-checker new-release-checker sale-checker all"
    
    # Available options
    local options="-b --build-only -h --help"
    
    # Check if we're in the first argument position (function name)
    if [[ ${COMP_CWORD} -eq 1 ]]; then
        # Complete function names for first argument
        COMPREPLY=( $(compgen -W "${functions}" -- ${cur}) )
        return 0
    fi
    
    # Check if previous argument was a function name
    case "${prev}" in
        paper-to-kindle-checker|new-release-checker|sale-checker|all)
            # Complete options after function name
            COMPREPLY=( $(compgen -W "${options}" -- ${cur}) )
            return 0
            ;;
        -b|--build-only|-h|--help)
            # No further completion after these options
            return 0
            ;;
        *)
            # Check if current word starts with dash (option)
            if [[ ${cur} == -* ]]; then
                COMPREPLY=( $(compgen -W "${options}" -- ${cur}) )
            else
                COMPREPLY=( $(compgen -W "${functions}" -- ${cur}) )
            fi
            return 0
            ;;
    esac
}

# Register the completion function for different ways to call the script
complete -F _deploy_completion ./scripts/deploy.sh
complete -F _deploy_completion scripts/deploy.sh
complete -F _deploy_completion deploy.sh

# Also register for when the script is in PATH
if [[ -x "./scripts/deploy.sh" ]]; then
    complete -F _deploy_completion "$(realpath ./scripts/deploy.sh)" 2>/dev/null || true
fi