#!/bin/sh
set -e

# Debug: Show annotation setting
echo "INPUT_ANNOTATIONS=$INPUT_ANNOTATIONS"

# Build args from environment variables
ARGS=""

if [ -n "$INPUT_COVERAGE_FILE" ]; then
    ARGS="$ARGS -coverage-file=$INPUT_COVERAGE_FILE"
fi

if [ -n "$INPUT_FORMAT" ]; then
    ARGS="$ARGS -format=$INPUT_FORMAT"
fi

if [ -n "$INPUT_SHOW_FILES" ]; then
    ARGS="$ARGS -show-files=$INPUT_SHOW_FILES"
fi

if [ -n "$INPUT_THRESHOLD" ]; then
    ARGS="$ARGS -threshold=$INPUT_THRESHOLD"
fi

if [ -n "$INPUT_TITLE" ]; then
    ARGS="$ARGS -title=$INPUT_TITLE"
fi

if [ "$INPUT_ANNOTATIONS" = "true" ]; then
    ARGS="$ARGS -annotations"
    echo "Annotations enabled"
fi

echo "Running: /litecov $ARGS"
exec /litecov $ARGS
