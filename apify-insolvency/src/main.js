// Apify Actor: German Insolvency Register
// Wraps the German Insolvency API and writes results to Apify dataset.
import { Actor } from 'apify';

await Actor.init();

const input = await Actor.getInput();
const {
    apiKey,
    apiBaseUrl = 'https://api.yourdomain.com',
    name = '',
    city = '',
    maxResults = 50,
} = input;

if (!apiKey) {
    throw new Error('apiKey is required. Get one at rapidapi.com');
}
if (!name && !city) {
    throw new Error('At least one of name or city is required');
}

const params = new URLSearchParams();
if (name) params.set('name', name);
if (city) params.set('city', city);
params.set('maxResults', String(maxResults));

const url = `${apiBaseUrl}/v1/insolvency/search?${params}`;
console.log(`Fetching: ${url}`);

const response = await fetch(url, {
    headers: {
        'X-API-Key': apiKey,
        'Accept': 'application/json',
    },
});

if (!response.ok) {
    const body = await response.text();
    throw new Error(`API error HTTP ${response.status}: ${body}`);
}

const data = await response.json();
const results = data.results || data.items || [];

console.log(`Found ${results.length} results (total: ${data.total ?? 'unknown'})`);

// Push all results to Apify dataset
await Actor.pushData(results);

console.log('Done. Results saved to dataset.');
await Actor.exit();
