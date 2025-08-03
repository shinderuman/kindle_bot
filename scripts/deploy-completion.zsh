#!/bin/zsh

# Zsh completion for deploy.sh script
_deploy_completion() {
    local context state line
    typeset -A opt_args
    
    # Define the completion specification
    _arguments -C \
        '1:function:(paper-to-kindle-checker new-release-checker sale-checker all)' \
        '*::options:->options' && return 0
    
    case $state in
        options)
            case $words[2] in
                paper-to-kindle-checker|new-release-checker|sale-checker|all)
                    _arguments \
                        '(-b --build-only)'{-b,--build-only}'[Only build, do not deploy]' \
                        '(-h --help)'{-h,--help}'[Show help message]'
                    ;;
            esac
            ;;
    esac
}

# Register the completion function
compdef _deploy_completion ./scripts/deploy.sh
compdef _deploy_completion scripts/deploy.sh
compdef _deploy_completion deploy.sh