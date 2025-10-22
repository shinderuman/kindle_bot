#!/bin/bash

# Build check script for kindle_bot project
# This script builds all commands to verify compilation without generating executables

echo "Building all commands..."

commands=("new-release-checker" "paper-to-kindle-checker" "sale-checker" "release-notifier")
failed_commands=()

for cmd in "${commands[@]}"; do
    echo "Building $cmd..."
    if ! go build -o /dev/null "./cmd/$cmd"; then
        echo "❌ Build failed for $cmd"
        failed_commands+=("$cmd")
    else
        echo "✅ Build successful for $cmd"
    fi
done

if [ ${#failed_commands[@]} -eq 0 ]; then
    echo ""
    echo "🎉 All builds completed successfully!"
    exit 0
else
    echo ""
    echo "❌ Build failed for the following commands:"
    for cmd in "${failed_commands[@]}"; do
        echo "  - $cmd"
    done
    exit 1
fi