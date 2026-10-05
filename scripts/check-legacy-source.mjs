#!/usr/bin/env node
// Read-only guard for the actual M5 legacy source; no install or generation.
import { createHash } from 'node:crypto';
import { readFileSync, readdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = process.argv[2]
  ? path.resolve(process.argv[2])
  : fileURLToPath(new URL('../', import.meta.url));
const manifest = path.join(root, 'docs/planning/legacy-m5-source.sha256');
const excluded = new Set(['web/dist', 'web/node_modules', 'web/coverage']);

function sourceFiles(directory) {
  const files = [];
  for (const entry of readdirSync(path.join(root, directory), { withFileTypes: true })) {
    const relative = `${directory}/${entry.name}`;
    if (excluded.has(relative)) continue;
    if (entry.isSymbolicLink()) throw new Error(`unexpected source symlink: ${relative}`);
    if (entry.isDirectory()) files.push(...sourceFiles(relative));
    else if (entry.isFile()) files.push(relative);
    else throw new Error(`unexpected source file type: ${relative}`);
  }
  return files.sort();
}

try {
  const expected = new Map();
  for (const line of readFileSync(manifest, 'utf8').split('\n')) {
    if (!line || line.startsWith('#')) continue;
    const match = /^([a-f0-9]{64})  (web\/.+)$/.exec(line);
    if (!match || match[2].split('/').some((part) => part === '..') || expected.has(match[2])) {
      throw new Error('invalid or duplicate legacy source manifest entry');
    }
    expected.set(match[2], match[1]);
  }
  if (!expected.size) throw new Error('empty legacy source manifest');
  const actual = new Set(sourceFiles('web'));
  const failures = [];
  for (const [file, digest] of expected) {
    if (!actual.has(file)) {
      failures.push(`missing: ${file}`);
      continue;
    }
    const actualDigest = createHash('sha256').update(readFileSync(path.join(root, file))).digest('hex');
    if (digest !== actualDigest) failures.push(`changed: ${file}`);
  }
  for (const file of actual) {
    if (!expected.has(file)) failures.push(`added: ${file}`);
  }
  if (failures.length) throw new Error(failures.join('\n'));
  console.log(`verified ${expected.size} frozen legacy source files`);
} catch (error) {
  console.error(`legacy source verification failed:\n${error.message}`);
  process.exitCode = 1;
}
