#!/bin/bash

# Load environment variables from .env file
if [ -f ../.env ]; then
    export $(grep -v '^#' ../.env | xargs)
fi

# Check if environment variable is set
if [ -z "$TELEGRAM_BOT_TOKEN" ]; then
    echo "Error: TELEGRAM_BOT_TOKEN is not set"
    exit 1
fi

# Set bot commands
curl -X POST "https://api.telegram.org/bot$TELEGRAM_BOT_TOKEN/setMyCommands" \
  -H "Content-Type: application/json" \
  -d '{
    "commands": [
      {
        "command": "rm",
        "description": "Remove a domain from monitoring"
      },
      {
        "command": "help",
        "description": "Show help message"
      }
    ]
  }'

echo "Bot commands configured successfully"