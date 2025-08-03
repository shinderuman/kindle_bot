#!/bin/bash

set -e

# Constants and variables
readonly SCRIPT_NAME="$(basename "$0")"

# Load environment variables from .env file
load_env() {
    if [ -f .env ]; then
        export $(grep -v '^#' .env | xargs)
    else
        echo "Error: .env file not found. Please create one based on .env.example"
        exit 1
    fi
}

# Function to show usage
show_usage() {
    cat << EOF
Usage: $SCRIPT_NAME <function> [--build-only]

Available functions:
  paper-to-kindle-checker   Deploy paper-to-kindle-checker
  new-release-checker       Deploy new-release-checker
  sale-checker              Deploy sale-checker
  all                       Deploy all functions

Options:
  -b, --build-only  Only build the function, don't deploy

Examples:
  $SCRIPT_NAME paper-to-kindle-checker
  $SCRIPT_NAME new-release-checker -b
  $SCRIPT_NAME all --build-only
EOF
}

# Function to build a Lambda function
build_function() {
    local source_path="$1"
    local function_name="$2"
    
    echo "Building $function_name from $source_path..."
    
    # Set environment variables for cross-compilation
    export GOOS=linux
    export GOARCH=amd64
    export CGO_ENABLED=0
    
    # Build the binary
    go build -ldflags="-s -w" -o bootstrap "$source_path"
    
    # Create zip file
    zip -r lambda.zip bootstrap
    
    echo "✅ Built $function_name successfully"
}

# Function to deploy a Lambda function
deploy_function() {
    local function_name="$1"
    
    echo "Deploying $function_name..."
    
    # Update Lambda function code and capture response
    local response
    response=$(aws lambda update-function-code \
        --profile "$AWS_PROFILE" \
        --function-name "$function_name" \
        --zip-file fileb://lambda.zip \
        --output json 2>&1)
    
    local exit_code=$?
    
    if [ $exit_code -eq 0 ]; then
        # Parse and display deployment information
        local code_size
        local last_modified
        local runtime
        local version
        
        # Parse JSON response using jq
        code_size=$(echo "$response" | jq -r '.CodeSize // "N/A"')
        last_modified=$(echo "$response" | jq -r '.LastModified // "N/A"')
        runtime=$(echo "$response" | jq -r '.Runtime // "N/A"')
        version=$(echo "$response" | jq -r '.Version // "N/A"')
        
        echo "✅ Deployed $function_name successfully"
        echo "   📦 Code Size: $(format_bytes "$code_size")"
        echo "   🕒 Last Modified: $last_modified"
        echo "   🔧 Runtime: $runtime"
        echo "   📋 Version: $version"
    else
        echo "❌ Failed to deploy $function_name"
        echo "Error details:"
        echo "$response"
        return 1
    fi
}

# Function to format bytes into human readable format
format_bytes() {
    local bytes="$1"
    
    if [ "$bytes" = "N/A" ] || [ -z "$bytes" ]; then
        echo "N/A"
        return
    fi
    
    if [ "$bytes" -lt 1024 ]; then
        echo "${bytes} B"
    elif [ "$bytes" -lt 1048576 ]; then
        echo "$((bytes / 1024)) KB"
    elif [ "$bytes" -lt 1073741824 ]; then
        echo "$((bytes / 1048576)) MB"
    else
        echo "$((bytes / 1073741824)) GB"
    fi
}

# Function to clean up temporary files
cleanup() {
    rm -f bootstrap lambda.zip
}

# Function to build and optionally deploy
process_function() {
    local source_path="$1"
    local function_name="$2"
    local build_only="$3"
    
    build_function "$source_path" "$function_name"
    
    if [ "$build_only" != "true" ]; then
        deploy_function "$function_name"
    fi
    
    cleanup
}

# Function to check if jq is installed
check_jq() {
    if ! command -v jq >/dev/null 2>&1; then
        echo "Error: jq is required but not installed"
        echo "Please install jq to parse JSON responses from AWS CLI"
        echo ""
        echo "Installation instructions:"
        echo "  macOS: brew install jq"
        echo "  Ubuntu/Debian: sudo apt-get install jq"
        echo "  CentOS/RHEL: sudo yum install jq"
        echo "  Or download from: https://stedolan.github.io/jq/download/"
        exit 1
    fi
}

# Function to validate AWS profile
validate_aws_profile() {
    if ! aws configure list-profiles | grep -q "^$AWS_PROFILE$"; then
        echo "Error: AWS profile '$AWS_PROFILE' not found"
        echo "Please configure the profile or update AWS_PROFILE in .env file"
        exit 1
    fi
}

# Function to parse command line arguments
parse_arguments() {
    FUNCTION=""
    BUILD_ONLY=false

    while [[ $# -gt 0 ]]; do
        case $1 in
            paper-to-kindle-checker)
                FUNCTION="paper-to-kindle-checker"
                shift
                ;;
            new-release-checker)
                FUNCTION="new-release-checker"
                shift
                ;;
            sale-checker)
                FUNCTION="sale-checker"
                shift
                ;;
            all)
                FUNCTION="all"
                shift
                ;;
            -b|--build-only)
                BUILD_ONLY=true
                shift
                ;;
            -h|--help)
                show_usage
                exit 0
                ;;
            *)
                echo "Unknown option: $1"
                show_usage
                exit 1
                ;;
        esac
    done

    # Check if function is specified
    if [ -z "$FUNCTION" ]; then
        echo "Error: No function specified"
        show_usage
        exit 1
    fi
}

# Function to deploy specific function based on selection
deploy_selected_function() {
    case $FUNCTION in
        paper-to-kindle-checker)
            process_function "cmd/paper-to-kindle-checker/main.go" "$PAPER_TO_KINDLE_CHECKER" "$BUILD_ONLY"
            ;;
        new-release-checker)
            process_function "cmd/new-release-checker/main.go" "$NEW_RELEASE_CHECKER" "$BUILD_ONLY"
            ;;
        sale-checker)
            process_function "cmd/sale-checker/main.go" "$SALE_CHECKER" "$BUILD_ONLY"
            ;;
        all)
            echo "Deploying all functions..."
            process_function "cmd/paper-to-kindle-checker/main.go" "$PAPER_TO_KINDLE_CHECKER" "$BUILD_ONLY"
            process_function "cmd/new-release-checker/main.go" "$NEW_RELEASE_CHECKER" "$BUILD_ONLY"
            process_function "cmd/sale-checker/main.go" "$SALE_CHECKER" "$BUILD_ONLY"
            ;;
    esac
}

# Main function
main() {
    # Load environment variables
    load_env
    
    # Parse command line arguments
    parse_arguments "$@"
    
    # Check required dependencies
    check_jq
    
    # Validate AWS profile
    validate_aws_profile
    
    # Display configuration
    echo "Using AWS profile: $AWS_PROFILE"
    if [ "$BUILD_ONLY" = true ]; then
        echo "Mode: Build only"
    else
        echo "Mode: Build and deploy"
    fi
    echo ""
    
    # Deploy selected function(s)
    deploy_selected_function
    
    echo ""
    echo "🎉 All operations completed successfully!"
}

# Execute main function with all arguments
main "$@"