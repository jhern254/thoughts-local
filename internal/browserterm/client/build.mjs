// Optional development task. The Go build embeds the committed output; Node is
// not required to build or run Thoughts. Never hand-edit generated terminal code.
import {readFile, writeFile} from 'node:fs/promises';
import {createHash} from 'node:crypto';
import {transform} from 'esbuild';

const root = new URL('../static/', import.meta.url);
const modules = [
  ['@xterm/xterm', 'xterm'],
  ['@xterm/addon-fit', 'addon-fit'],
  ['@xterm/addon-unicode-graphemes', 'addon-unicode-graphemes']
];
const hashes = [];
let notices = 'Embedded terminal assets\n========================\n\n';
for (const [pkg, name] of modules) {
  const base = new URL(`node_modules/${pkg}/`, import.meta.url);
  const metadata = JSON.parse(await readFile(new URL('package.json', base), 'utf8'));
  const upstream = await readFile(new URL(`lib/${name}.js`, base), 'utf8');
  // Some vendored xterm utilities bypass its configurable logger. Remove their
  // console calls reproducibly instead of overriding the browser's console.
  // Do not minify identifiers again: upstream's published bundle is minified.
  const {code} = await transform(upstream, {drop: ['console'], minify: false, legalComments: 'inline', target: 'es2022'});
  await writeFile(new URL(`${name}.js`, root), code);
  hashes.push(`${createHash('sha256').update(code).digest('hex')}  ${name}.js`);
  notices += `${pkg} ${metadata.version}\nSource commit: ${metadata.commit}\n\n${await readFile(new URL('LICENSE', base), 'utf8')}\n\n`;
}
const css = await readFile(new URL('node_modules/@xterm/xterm/css/xterm.css', import.meta.url));
const terminalSource = await readFile(new URL('node_modules/@xterm/xterm/src/common/CoreTerminal.ts', import.meta.url), 'utf8');
notices += 'Additional terminal attribution (from upstream CoreTerminal.ts)\n' + terminalSource.slice(0, terminalSource.indexOf('*/') + 2) + '\n';
await writeFile(new URL('xterm.css', root), css);
hashes.push(`${createHash('sha256').update(css).digest('hex')}  xterm.css`);
await writeFile(new URL('../ASSETS.sha256', import.meta.url), hashes.join('\n')+'\n');
await writeFile(new URL('LICENSES.txt', root), notices);
