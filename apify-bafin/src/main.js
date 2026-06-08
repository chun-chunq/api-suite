// Apify Actor: BaFin Licensed Institutions
import { Actor } from 'apify';

await Actor.init();

const input = await Actor.getInput();
const {
    apiKey,
    apiBaseUrl = 'https://api.yourdomain.com',
    name = '',
    licenseType = '',
    maxResults = 50,
} = input;

if (!apiKey) throw new Error('apiKey is required');
if (!name && !licenseType) throw new Error('At least one of name or licenseType is required');

const params = new URLSearchParams();
if (name) params.set('name', name);
if (licenseType) params.set('licenseType', licenseType);
params.set('maxResults', String(maxResults));

const url = `${apiBaseUrl}/v1/bafin/search?${params}`;
console.log(`Fetching: ${url}`);

const resp = await fetch(url, {
    headers: { 'X-API-Key': apiKey, 'Accept': 'application/json' },
});

if (!resp.ok) throw new Error(`API error HTTP ${resp.status}: ${await resp.text()}`);
const data = await resp.json();
const results = data.results ?? [];

console.log(`Found ${results.length} institutions`);
await Actor.pushData(results);
await Actor.exit();
