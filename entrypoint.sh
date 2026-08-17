#!/bin/sh
set -e

# Build args from environment variables. Each one is appended as its own
# positional parameter via `set --`, never through string concatenation, so
# an input value is passed as argv and can't be re-parsed as shell source or
# split on whitespace.
set --

if [ -n "$INPUT_COVERAGE_FILE" ]; then
    set -- "$@" "-coverage-file=$INPUT_COVERAGE_FILE"
fi

if [ -n "$INPUT_FORMAT" ]; then
    set -- "$@" "-format=$INPUT_FORMAT"
fi

if [ -n "$INPUT_SHOW_FILES" ]; then
    set -- "$@" "-show-files=$INPUT_SHOW_FILES"
fi

if [ -n "$INPUT_THRESHOLD" ]; then
    set -- "$@" "-threshold=$INPUT_THRESHOLD"
fi

if [ -n "$INPUT_PATCH_THRESHOLD" ]; then
    set -- "$@" "-patch-threshold=$INPUT_PATCH_THRESHOLD"
fi

if [ -n "$INPUT_TITLE" ]; then
    set -- "$@" "-title=$INPUT_TITLE"
fi

if [ "$INPUT_ANNOTATIONS" = "true" ]; then
    set -- "$@" "-annotations=true"
fi

exec /litecov "$@"
