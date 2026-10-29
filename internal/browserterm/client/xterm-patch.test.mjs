import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {transform} from 'esbuild';
import {trimAlternateScreenOnShrink} from './xterm-patch.mjs';

test('pinned xterm patch accepts exactly the reviewed bundle and refuses upgrades',async()=>{
 const base=new URL('node_modules/@xterm/xterm/',import.meta.url);
 const metadata=JSON.parse(await readFile(new URL('package.json',base),'utf8'));
 const {code}=await transform(await readFile(new URL('lib/xterm.js',base),'utf8'),{drop:['console'],minify:false,legalComments:'inline',target:'es2022'});
 const patched=trimAlternateScreenOnShrink(code,metadata.version);
 assert.equal(patched,await readFile(new URL('../static/xterm.js',import.meta.url),'utf8'));
 for(const version of ['6.0.1','7.0.0']) assert.throws(()=>trimAlternateScreenOnShrink(code,version),/Review xterm viewport patch for the new version/);
 for(const changed of ['',code+code,patched]) assert.throws(()=>trimAlternateScreenOnShrink(changed,'6.0.0'),/resize source changed/);
});
