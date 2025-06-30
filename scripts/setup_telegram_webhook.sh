#!/bin/bash

# Load environment variables from .env file
if [ -f ../.env ]; then
    export $(grep -v '^#' ../.env | xargs)
fi

# Check if environment variables are set
if [ -z "$TELEGRAM_BOT_TOKEN" ]; then
    echo "Error: TELEGRAM_BOT_TOKEN is not set"
    exit 1
fi

if [ -z "$TELEGRAM_WEBHOOK_URL" ]; then
    echo "Error: TELEGRAM_WEBHOOK_URL is not set"
    exit 1
fi

# Set webhook
curl -X POST "https://api.telegram.org/bot$TELEGRAM_BOT_TOKEN/setWebhook" \
  -H "Content-Type: application/json" \
  -d "{\"url\":\"$TELEGRAM_WEBHOOK_URL\"}"

echo "Webhook set to: $TELEGRAM_WEBHOOK_URL"