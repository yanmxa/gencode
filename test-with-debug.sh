#!/bin/bash
# Test GenCode with DEBUG_SCHEMA enabled

export DEBUG_SCHEMA=1
export DEBUG_TOKENS=1

# Run GenCode and immediately send a test prompt
echo "Use the Task tool to explore authentication patterns" | npm start
