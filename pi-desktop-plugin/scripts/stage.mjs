import { cpSync, existsSync, mkdirSync, readFileSync } from 'node:fs';
import { dirname, isAbsolute, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

// Stage source only; never inspect user data, run Deja or copy an executable.
export function stage(output) {
  if (!isAbsolute(output)) throw new Error('输出目录必须是绝对路径');
  if (existsSync(output)) throw new Error('输出目录已存在，拒绝覆盖');
  const source = resolve(dirname(fileURLToPath(import.meta.url)), '..');
  mkdirSync(output, { recursive: true });
  for (const file of ['manifest.json', 'package.json', 'main.cjs', 'README.md', 'VERIFICATION.md', 'LICENSE.deja-vu', 'src']) {
    cpSync(join(source, file), join(output, file), { recursive: true, errorOnExist: true, force: false });
  }
  return JSON.parse(readFileSync(join(output, 'manifest.json'), 'utf8'));
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    if (process.argv.length !== 3) throw new Error('用法：node scripts/stage.mjs <新的绝对输出目录>');
    stage(process.argv[2]); console.log('已组装源码插件；未复制 EXE、未读取或索引历史。请用官方 PluginCheck / PluginPack。');
  } catch (error) { console.error(error.message); process.exitCode = 1; }
}
