#!/bin/bash
# Bump VERSION in malten.js - the single source of truth

cd "$(dirname "$0")"

# Get current version from malten.js
CURRENT=$(grep -oP '^var VERSION = \K\d+' client/web/malten.js)
NEXT=$((CURRENT + 1))

# Update VERSION in malten.js
sed -i "s/^var VERSION = $CURRENT;/var VERSION = $NEXT;/" client/web/malten.js

echo "Bumped VERSION: $CURRENT → $NEXT"
