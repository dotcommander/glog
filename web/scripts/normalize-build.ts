const indexURL = new URL('../build/index.html', import.meta.url);
const index = await Bun.file(indexURL).text();

// SvelteKit leaves indentation on an otherwise empty head line. Generated
// artifacts stay reviewable by normalizing whitespace after every build.
await Bun.write(indexURL, index.replace(/^[\t ]+$/gm, ''));
