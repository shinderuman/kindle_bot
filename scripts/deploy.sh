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
    
    # Get current function info before deployment
    local current_info
    current_info=$(aws lambda get-function \
        --profile "$AWS_PROFILE" \
        --function-name "$function_name" \
        --output json 2>/dev/null || echo "{}")
    
    local old_code_size
    old_code_size=$(echo "$current_info" | jq -r '.Configuration.CodeSize // 0')
    
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
        local code_size last_modified runtime version code_sha256 memory_size timeout
        
        # Parse JSON response using jq
        code_size=$(echo "$response" | jq -r '.CodeSize // "N/A"')
        last_modified=$(echo "$response" | jq -r '.LastModified // "N/A"')
        runtime=$(echo "$response" | jq -r '.Runtime // "N/A"')
        version=$(echo "$response" | jq -r '.Version // "N/A"')
        code_sha256=$(echo "$response" | jq -r '.CodeSha256 // "N/A"')
        
        # Get memory and timeout from the update response (they're included there)
        memory_size=$(echo "$response" | jq -r '.MemorySize // "N/A"')
        timeout=$(echo "$response" | jq -r '.Timeout // "N/A"')
        
        # Calculate size difference
        local size_diff=""
        if [ "$old_code_size" != "0" ] && [ "$code_size" != "N/A" ]; then
            local diff=$((code_size - old_code_size))
            if [ $diff -gt 0 ]; then
                size_diff=" (+$(format_bytes $diff))"
            elif [ $diff -lt 0 ]; then
                size_diff=" ($(format_bytes $diff))"
            else
                size_diff=" (no change)"
            fi
        fi
        
        echo "✅ Deployed $function_name successfully"
        echo "   📦 Code Size: $(format_bytes "$code_size")$size_diff"
        echo "   🕒 Last Modified: $(format_timestamp "$last_modified")"
        echo "   🔧 Runtime: $runtime"
        echo "   💾 Memory: ${memory_size} MB"
        echo "   ⏱️  Timeout: $(format_timeout "$timeout")"
        echo "   🔑 SHA256: ${code_sha256:0:12}..."
        echo "   📋 Version: $version"
        
        # Get function URL if exists
        local function_url
        function_url=$(aws lambda get-function-url-config \
            --profile "$AWS_PROFILE" \
            --function-name "$function_name" \
            --output json 2>/dev/null | jq -r '.FunctionUrl // empty')
        
        if [ -n "$function_url" ]; then
            echo "   🌐 Function URL: $function_url"
        fi
        
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
    
    # Handle negative numbers for size differences
    local sign=""
    if [ "$bytes" -lt 0 ]; then
        sign="-"
        bytes=$((bytes * -1))
    fi
    
    if [ "$bytes" -lt 1024 ]; then
        echo "${sign}${bytes} B"
    elif [ "$bytes" -lt 1048576 ]; then
        echo "${sign}$((bytes / 1024)) KB"
    elif [ "$bytes" -lt 1073741824 ]; then
        echo "${sign}$((bytes / 1048576)) MB"
    else
        echo "${sign}$((bytes / 1073741824)) GB"
    fi
}

# Function to format timestamp into readable format (UTC)
format_timestamp() {
    local timestamp="$1"
    
    if [ "$timestamp" = "N/A" ] || [ -z "$timestamp" ]; then
        echo "N/A"
        return
    fi
    
    # Convert ISO 8601 timestamp to readable UTC format
    local clean_timestamp="${timestamp%.*}"  # Remove milliseconds
    clean_timestamp="${clean_timestamp%+*}"  # Remove timezone info
    clean_timestamp="${clean_timestamp}Z"    # Add UTC indicator
    
    # Simple format conversion: 2025-08-10T16:14:58Z -> 2025-08-10 16:14:58 UTC
    echo "${clean_timestamp}" | sed 's/T/ /' | sed 's/Z/ UTC/'
}

# Function to format timeout into readable format
format_timeout() {
    local seconds="$1"
    
    if [ "$seconds" = "N/A" ] || [ -z "$seconds" ]; then
        echo "N/A"
        return
    fi
    
    local minutes=$((seconds / 60))
    local remaining_seconds=$((seconds % 60))
    
    if [ $minutes -gt 0 ]; then
        if [ $remaining_seconds -gt 0 ]; then
            echo "${minutes}m ${remaining_seconds}s (${seconds}s)"
        else
            echo "${minutes}m (${seconds}s)"
        fi
    else
        echo "${seconds}s"
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
    
    # Display summary information
    if [ "$BUILD_ONLY" != true ]; then
        echo ""
        echo "📊 Deployment Summary:"
        echo "   🔗 AWS Console: https://console.aws.amazon.com/lambda/home?region=$(aws configure get region --profile "$AWS_PROFILE" 2>/dev/null || echo "us-east-1")#/functions"
        echo "   📈 CloudWatch Logs: https://console.aws.amazon.com/cloudwatch/home?region=$(aws configure get region --profile "$AWS_PROFILE" 2>/dev/null || echo "us-east-1")#logsV2:log-groups"
        echo "   ⚡ CloudWatch Events: https://console.aws.amazon.com/events/home?region=$(aws configure get region --profile "$AWS_PROFILE" 2>/dev/null || echo "us-east-1")#/rules"
        
        # Show next steps
        echo ""
        echo "💡 Next Steps:"
        echo "   • Monitor function execution in CloudWatch Logs"
        echo "   • Check CloudWatch Events for scheduled triggers"
        echo "   • Verify function configurations in AWS Console"
        echo "   • Test functions manually if needed"
    fi
}

# Execute main function with all arguments
main "$@"