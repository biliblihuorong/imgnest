import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

const checker = fileURLToPath(new URL('./check-legacy-source.mjs', import.meta.url));

function fixture(t) {
  const root = mkdtempSync(path.join(tmpdir(), 'imgnest-legacy-guard-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  mkdirSync(path.join(root, 'web/src'), { recursive: true });
  mkdirSync(path.join(root, 'docs/planning'), { recursive: true });
  const content = 'frozen M5 source\n';
  writeFileSync(path.join(root, 'web/src/main.ts'), content);
  writeFileSync(path.join(root, 'docs/planning/legacy-m5-source.sha256'),
    `${createHash('sha256').update(content).digest('hex')}  web/src/main.ts\n`);
  return root;
}

function check(root) {
  return spawnSync(process.execPath, [checker, root], { encoding: 'utf8' });
}

test('accepts the frozen source and ignores generated assets/dependencies', (t) => {
  const root = fixture(t);
  for (const dir of ['web/dist', 'web/node_modules', 'web/coverage']) {
    mkdirSync(path.join(root, dir), { recursive: true });
    writeFileSync(path.join(root, dir, 'generated.js'), 'generated');
  }
  const result = check(root);
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /verified 1 frozen legacy source files/);
});

test('rejects changed protected source content', (t) => {
  const root = fixture(t);
  writeFileSync(path.join(root, 'web/src/main.ts'), 'modified');
  const result = check(root);
  assert.equal(result.status, 1);
  assert.match(result.stderr, /changed: web\/src\/main.ts/);
});

test('rejects a missing protected source file', (t) => {
  const root = fixture(t);
  rmSync(path.join(root, 'web/src/main.ts'));
  const result = check(root);
  assert.equal(result.status, 1);
  assert.match(result.stderr, /missing: web\/src\/main.ts/);
});

test('rejects new source files not present at the M5 baseline', (t) => {
  const root = fixture(t);
  writeFileSync(path.join(root, 'web/src/new.ts'), 'new source');
  const result = check(root);
  assert.equal(result.status, 1);
  assert.match(result.stderr, /added: web\/src\/new.ts/);
});
