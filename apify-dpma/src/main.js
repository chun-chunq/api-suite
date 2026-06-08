// Apify Actor: DPMA German Trademark Register
import { Actor } from 'apify';

await Actor.init();

const input = await Actor.getInput();
const {
    apiKey,
    apiBaseUrl = 'https://api.yourdomain.com',
    name = '',
    owner = '',
    class: niceClass = '',
    status = '',
    maxResults = 50,
} = input;

if (!apiKey) throw new Error('apiKey is required');
if (!name && !owner) throw new Error('At least one of name or owner is required');

const params = new URLSearchParams();
if (name) params.set('name', name);
if (owner) params.set('owner', owner);
if (niceClass) params.set('class', niceClass);
if (status) params.set('status', status);
params.set('maxResults', String(maxResults));

const url = `${apiBaseUrl}/v1/trademark/search?${params}`;
console.log(`Fetching: ${url}`);

const resp = await fetch(url, {
    headers: { 'X-API-Key': apiKey, 'Accept': 'application/json' },
});

if (!resp.ok) throw new Error(`API error HTTP ${resp.status}: ${await resp.text()}`);
const data = await resp.json();
const results = data.results ?? [];

console.log(`Found ${results.length} trademarks (total: ${data.total ?? '?'})`);
await Actor.pushData(results);
await Actor.exit();
