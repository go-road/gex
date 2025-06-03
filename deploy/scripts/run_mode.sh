#!/bin/bash

# Check if running in local mode
if [ "$1" = "local" ]; then
    echo "Starting in local development mode..."
    make run-infra
    make run-local
    exit 0
fi
echo "Starting in container mode..."
make run




