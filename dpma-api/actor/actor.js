// Apify Actor — DPMA Trademark Search
// Reads input from Apify platform and calls the dpma-api.
// Deploy via: apify push (from this directory)

import fetch from 'node-fetch';

const API_URL = process.env.DPMA_API_URL || 'https://your-server.de';
const API_KEY = process.env.DPMA_API_KEY || 'demo-key-12345';

// Apify provides INPUT as env var (JSON string)
const input = JSON.parse(process.env.ACTOR_INPUT || process.env.INPUT || '{}');

console.log('DPMA Trademark Actor starting with input:', JSON.stringify(input));

async function run() {
    const response = await fetch(`${API_URL}/apify/run`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-API-Key': API_KEY },
        body: JSON.stringify(input),
    });

    if (!response.ok) {
        console.error('API error:', response.status, await response.text());
        process.exit(1);
    }

    const result = await response.json();

    // Apify expects output written to stdout in a specific format
    console.log('OUTPUT:', JSON.stringify(result));

    // Write to Apify key-value store if running on platform
    if (process.env.APIFY_DEFAULT_KEY_VALUE_STORE_ID) {
        const { Actor } = await import('apify');
        await Actor.setValue('OUTPUT', result);
        await Actor.pushData(result.output?.results || []);
    }

    console.log(`Done. Found ${result.output?.totalCount || 0} trademarks.`);
}

run().catch(err => {
    console.error('Actor failed:', err.message);
    process.exit(1);
});
