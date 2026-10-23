import { accessSync, constants, mkdirSync, readFileSync, renameSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { delimiter, dirname, isAbsolute, join } from 'node:path';
import { homedir } from 'node:os';
import { randomUUID } from 'node:crypto';

// One allowlist and one persistent configuration for hooks and the MCP bridge.
// Ambient DEJA_*/XDG_* are deliberately not read: GUI and Agent environments differ.
export const sharedKeys = new Set([
  'DEJA_STORES', 'DEJA_EXCLUDE_PROJECTS', 'DEJA_EXCLUDE_HARNESSES', 'DEJA_AUTORECALL_LOCAL_ONLY', 'DEJA_SEARCH_ONLY', 'DEJA_RECALL', 'DEJA_NO_AUTO_INDEX',
  'DEJA_PI_DESKTOP_ROOT', 'DEJA_CLAUDE_ROOT', 'DEJA_CLAUDE_HISTORY_FILE', 'DEJA_CODEX_ROOT',
  'DEJA_OPENCODE_DB', 'DEJA_OPENCODE_DIFFS', 'DEJA_AIDER_ROOTS', 'DEJA_AMP_ROOT', 'DEJA_ANTIGRAVITY_ROOT',
  'DEJA_CLINE_ROOT', 'DEJA_CLINE_ROOTS', 'DEJA_CURSOR_ROOT', 'DEJA_CURSOR_CLI_ROOT', 'DEJA_DEEPSEEK_ROOT',
  'DEJA_GEMINI_ROOT', 'DEJA_GOOSE_ROOT', 'DEJA_GOOSE_DB', 'DEJA_GROK_ROOT', 'DEJA_GROK_DB',
  'DEJA_HERMES_HOME', 'DEJA_HERMES_PROFILES_ROOT', 'DEJA_HERMES_DB', 'DEJA_KIMI_ROOT', 'DEJA_KIMCHI_ROOT',
  'DEJA_OMP_ROOT', 'DEJA_OPENCLAW_ROOT', 'DEJA_CONTINUE_ROOT', 'DEJA_CRUSH_ROOT', 'DEJA_PI_ROOT',
  'DEJA_PRIME_ROOT', 'DEJA_QWEN_ROOT', 'DEJA_CHERRYSTUDIO_ROOTS', 'DEJA_SENPI_ROOT', 'DEJA_GJC_ROOT',
  'DEJA_COMMANDCODE_ROOT', 'DEJA_ZCODE_ROOT', 'DEJA_ZCODE_DB', 'DEJA_ZCODE_LEGACY_ROOT',
  'DEJA_KIRO_ROOT', 'DEJA_KIRO_DB', 'DEJA_KILO_ROOTS', 'DEJA_KILO_DB', 'DEJA_ROO_ROOTS', 'DEJA_ROO_CLI_ROOT',
  'DEJA_ZED_ROOT', 'DEJA_ZED_DB', 'DEJA_DEVIN_DB', 'DEJA_CODEWHALE_ROOT', 'DEJA_JUNIE_ROOT', 'DEJA_JETBRAINS_ROOT',
  'DEJA_WINDSURF_ROOT', 'DEJA_CODEBUDDY_ROOTS', 'DEJA_REASONIX_ROOT', 'DEJA_TRAE_ROOT', 'DEJA_MUSE_ROOTS',
  'DEJA_COPILOT_ROOT', 'DEJA_COPILOT_CHAT_ROOTS',
  'DEJA_INDEX_DIR', 'DEJA_NOTES_FILE', 'DEJA_POLICY_FILE', 'DEJA_SOURCE_PATHS_FILE',
  'XDG_CONFIG_HOME', 'XDG_DATA_HOME', 'XDG_CACHE_HOME', 'XDG_STATE_HOME',
]);
const pathKeys = new Set(['DEJA_INDEX_DIR', 'DEJA_NOTES_FILE', 'DEJA_POLICY_FILE', 'DEJA_SOURCE_PATHS_FILE',
  'XDG_CONFIG_HOME', 'XDG_DATA_HOME', 'XDG_CACHE_HOME', 'XDG_STATE_HOME']);
export function configLocation(environment = process.env, home = (process.platform === 'win32' ? environment.USERPROFILE : environment.HOME) || homedir()) {
  const file = environment.DEJA_PI_CONFIG_FILE || join(home, '.config', 'deja', 'pi-desktop.json');
  if (!isAbsolute(file)) throw new Error('DEJA_PI_CONFIG_FILE 必须是绝对路径');
  return file;
}
export function validateSettings(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value) || value.version !== 1) throw new Error('插件配置需要 version: 1');
  for (const key of Object.keys(value)) if (!['version', 'executable', 'autoIndex', 'environment'].includes(key)) throw new Error(`未知插件配置项：${key}`);
  if (value.executable !== undefined && (typeof value.executable !== 'string' || (value.executable && !isAbsolute(value.executable)))) throw new Error('Deja 程序必须是绝对路径；空字符串表示自动查找');
  if (value.autoIndex !== undefined && typeof value.autoIndex !== 'boolean') throw new Error('autoIndex 必须为布尔值');
  if (value.environment !== undefined && (!value.environment || typeof value.environment !== 'object' || Array.isArray(value.environment))) throw new Error('environment 必须是对象');
  for (const [key, item] of Object.entries(value.environment ?? {})) {
    if (!sharedKeys.has(key) || typeof item !== 'string' || item.includes('\0')) throw new Error(`不允许的共享配置项：${key}`);
    if (pathKeys.has(key) && !isAbsolute(item)) throw new Error(`${key} 必须是绝对路径`);
  }
  return { version: 1, executable: '', autoIndex: false, environment: {}, ...value };
}
export function readSettings(file) {
  try {
    if (statSync(file).size > 64 * 1024) throw new Error('配置超过 64 KiB');
    return validateSettings(JSON.parse(readFileSync(file, 'utf8')));
  } catch (error) {
    if (error.code === 'ENOENT') return validateSettings({ version: 1 });
    throw new Error(`无法读取配置 ${file}：${error.message}`);
  }
}
export function saveSettings(file, patch) {
  mkdirSync(dirname(file), { recursive: true });
  const lock = `${file}.lock`, temporary = `${file}.${randomUUID()}.tmp`;
  try { writeFileSync(lock, String(process.pid), { flag: 'wx', mode: 0o600 }); }
  catch (error) { throw new Error(`配置正被其他进程修改，请稍后重试（${lock}）：${error.message}`); }
  try {
    const next = validateSettings({ ...readSettings(file), ...patch, version: 1 });
    const content = JSON.stringify(next, null, 2) + '\n';
    if (Buffer.byteLength(content) > 64 * 1024) throw new Error('配置超过 64 KiB');
    writeFileSync(temporary, content, { flag: 'wx', mode: 0o600 });
    renameSync(temporary, file);
    return next;
  } finally { rmSync(temporary, { force: true }); rmSync(lock, { force: true }); }
}
function nativeFile(file) {
  if (!isAbsolute(file)) return false;
  if (process.platform === 'win32' && !file.toLowerCase().endsWith('.exe')) return false;
  try { if (!statSync(file).isFile()) return false; accessSync(file, process.platform === 'win32' ? constants.F_OK : constants.X_OK); return true; }
  catch { return false; }
}
export function findExecutable(name, environment = process.env, home = homedir()) {
  const filename = process.platform === 'win32' ? `${name}.exe` : name;
  const paths = (environment.PATH || environment.Path || '').split(delimiter).filter(path => isAbsolute(path));
  const extra = name === 'deja' ? [join(home, '.local', 'bin'), join(home, 'bin'), join(home, 'go', 'bin'),
    ...(process.platform === 'win32' ? [join(home, 'scoop', 'shims')] : ['/opt/homebrew/bin', '/usr/local/bin', '/usr/bin'])] : [];
  return [...paths, ...extra].map(path => join(path, filename)).find(nativeFile) || '';
}
export function resolveRuntime({ environment = process.env, home = (process.platform === 'win32' ? environment.USERPROFILE : environment.HOME) || homedir(), file = configLocation(environment, home) } = {}) {
  const settings = readSettings(file), values = settings.environment;
  const configHome = values.XDG_CONFIG_HOME || join(home, '.config');
  const notesHome = values.XDG_DATA_HOME || (process.platform === 'win32' ? environment.APPDATA || join(home, 'AppData', 'Roaming') : join(home, '.local', 'share'));
  const manual = (values.DEJA_NO_AUTO_INDEX || '').trim().toLowerCase();
  const env = {
    ...values, HOME: home, USERPROFILE: home,
    ...(environment.APPDATA ? { APPDATA: environment.APPDATA } : {}),
    PATH: environment.PATH || environment.Path || '', XDG_CONFIG_HOME: configHome,
    DEJA_INDEX_DIR: values.DEJA_INDEX_DIR || join(home, '.cache', 'deja', 'index.db'),
    DEJA_NOTES_FILE: values.DEJA_NOTES_FILE || join(notesHome, 'deja', 'notes.jsonl'),
    DEJA_POLICY_FILE: values.DEJA_POLICY_FILE || join(configHome, 'deja', 'policy.json'),
    DEJA_SOURCE_PATHS_FILE: values.DEJA_SOURCE_PATHS_FILE || join(home, '.config', 'deja', 'source-paths.json'),
    DEJA_OFFLINE: '1', DEJA_NO_AUTO_INDEX: '1', DEJA_HOOK_REFRESH: '1',
    DEJA_PI_AUTO_INDEX: !settings.autoIndex ? '0' : !manual ? 'policy' : ['0', 'false', 'no', 'off'].includes(manual) ? '1' : '0',
  };
  const executable = settings.executable ? (nativeFile(settings.executable) ? settings.executable : '') : findExecutable('deja', environment, home);
  return { executable, env, settings, configFile: file, selectedExecutable: settings.executable,
    sqliteExecutable: findExecutable('sqlite3', environment, home), error: undefined };
}
export function loadRuntime(options = {}) {
  const config = { configFile: options.file || configLocation(options.environment, options.home) };
  config.refresh = () => { Object.assign(config, resolveRuntime(options)); return config; };
  // Keep /deja available even when the user's configuration is malformed.
  try { config.refresh(); } catch (error) { config.error = error.message; }
  return config;
}
