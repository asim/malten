#!/bin/bash
# Bump JS version in both index.html and malten.js

cd "$(dirname "$0")"

# Get current version from index.html
CURRENT=$(grep -oP 'malten\.js\?v=\K\d+' client/web/index.html)
NEXT=$((CURRENT + 1))

# Update both files
sed -i "s/malten\.js?v=$CURRENT/malten.js?v=$NEXT/" client/web/index.html
sed -i "s/JS version: $CURRENT/JS version: $NEXT/" client/web/malten.js

echo "Bumped JS version: $CURRENT → $NEXT"
