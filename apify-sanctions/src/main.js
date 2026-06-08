// Apify Actor: EU Sanctions Screening
import { Actor } from 'apify';

await Actor.init();

const input = await Actor.getInput();
const {
    apiKey,
    apiBaseUrl = 'https://api.yourdomain.com',
    names = [],
    mode = 'check',
} = input;

if (!apiKey) throw new Error('apiKey is required');
if (!names.length) throw new Error('names array is required and must not be empty');

const endpoint = mode === 'search' ? 'search' : 'check';
const results = [];

for (const name of names) {
    const url = `${apiBaseUrl}/v1/sanctions/${endpoint}?q=${encodeURIComponent(name)}&maxResults=10`;
    console.log(`Screening: "${name}"`);

    const resp = await fetch(url, {
        headers: { 'X-API-Key': apiKey, 'Accept': 'application/json' },
    });

    if (!resp.ok) {
        console.error(`Error screening "${name}": HTTP ${resp.status}`);
        results.push({ query: name, error: `HTTP ${resp.status}` });
        continue;
    }

    const data = await resp.json();
    if (mode === 'check') {
        results.push({
            query: name,
            matched: data.matched ?? (data.count > 0),
            count: data.count ?? 0,
            matches: data.matches ?? data.results ?? [],
        });
    } else {
        results.push({
            query: name,
            count: data.count ?? data.total ?? 0,
            results: data.results ?? [],
        });
    }
}

await Actor.pushData(results);
console.log(`Done. Screened ${names.length} names.`);
await Actor.exit();
