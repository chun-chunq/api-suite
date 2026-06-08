# Apify Actor wrapper for the DPMA Trademark API.
# Deploy this on Apify: https://apify.com/actors
#
# Input schema: actor/INPUT_SCHEMA.json
# The actor reads input from APIFY_INPUT env var and calls our API.

FROM node:20-slim
WORKDIR /app

# Simple Node.js wrapper that reads Apify INPUT and calls our API
COPY actor.js .
RUN npm init -y && npm install node-fetch@3

CMD ["node", "actor.js"]
