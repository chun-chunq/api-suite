// Apify Actor: GLEIF LEI Lookup
import { Actor } from 'apify';

await Actor.init();

const input = await Actor.getInput();
const {
    apiKey,
    apiBaseUrl = 'https://api.yourdomain.com',
    mode = 'search',
    companyName = '',
    country = '',
    lei = '',
    leiCodes = [],
    includeRelationships = false,
    activeOnly = true,
} = input;

if (!apiKey) throw new Error('apiKey is required');

async function apiGet(path) {
    const resp = await fetch(`${apiBaseUrl}${path}`, {
        headers: { 'X-API-Key': apiKey, 'Accept': 'application/json' },
    });
    if (!resp.ok) throw new Error(`HTTP ${resp.status}: ${await resp.text()}`);
    return resp.json();
}

const results = [];

if (mode === 'search') {
    if (!companyName) throw new Error('companyName is required in search mode');
    const params = new URLSearchParams({ name: companyName, limit: '100', active: String(activeOnly) });
    if (country) params.set('country', country);
    const data = await apiGet(`/v1/lei/search?${params}`);
    const entities = data.results ?? [];
    console.log(`Found ${entities.length} entities (total: ${data.total})`);
    for (const e of entities) {
        const record = { ...e };
        if (includeRelationships) {
            try {
                const rel = await apiGet(`/v1/lei/${e.lei}/relationships`);
                record.relationships = rel.relationships;
            } catch { /* skip */ }
        }
        results.push(record);
    }

} else if (mode === 'lookup') {
    if (!lei) throw new Error('lei is required in lookup mode');
    const data = await apiGet(`/v1/lei/${lei}`);
    const record = data.lei ?? data;
    if (includeRelationships) {
        try {
            const rel = await apiGet(`/v1/lei/${lei}/relationships`);
            record.relationships = rel.relationships;
        } catch { /* skip */ }
    }
    results.push(record);

} else if (mode === 'bulk_lookup') {
    if (!leiCodes.length) throw new Error('leiCodes array is required in bulk_lookup mode');
    for (const code of leiCodes) {
        try {
            const data = await apiGet(`/v1/lei/${code}`);
            const record = data.lei ?? data;
            if (includeRelationships) {
                try {
                    const rel = await apiGet(`/v1/lei/${code}/relationships`);
                    record.relationships = rel.relationships;
                } catch { /* skip */ }
            }
            results.push(record);
        } catch (e) {
            results.push({ lei: code, error: e.message });
        }
    }
}

await Actor.pushData(results);
console.log(`Done. ${results.length} records saved.`);
await Actor.exit();
