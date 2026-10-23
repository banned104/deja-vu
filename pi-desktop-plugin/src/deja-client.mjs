import { spawn } from 'node:child_process';
import { readFile, stat } from 'node:fs/promises';
import { isAbsolute, join } from 'node:path';
import { loadRuntime } from './runtime-config.mjs';

export const loadConfig = loadRuntime;

// Direct Node children are not managed by the host exec API (which has no
// stdin). Own a process group/tree here and wait for close before settling.
export function runProcess(executable, args, { input = '', cwd, env = {}, signal,
  timeout = 1500, maxBytes = 64 * 1024 } = {}) {
  return new Promise((resolveResult, reject) => {
    if (signal?.aborted) return reject(new Error('Deja-vu request cancelled'));
    const childEnv = {};
    for (const key of ['PATH', 'Path', 'HOME', 'USERPROFILE', 'APPDATA', 'LOCALAPPDATA', 'SystemRoot', 'SYSTEMROOT', 'WINDIR', 'ComSpec', 'COMSPEC', 'PATHEXT', 'TEMP', 'TMP', 'TMPDIR', 'LANG', 'LC_ALL']) {
      if (process.env[key]) childEnv[key] = process.env[key];
    }
    Object.assign(childEnv, env, { DEJA_OFFLINE: '1' });
    const child = spawn(executable, args, {
      cwd, env: childEnv, stdio: ['pipe', 'pipe', 'pipe'],
      windowsHide: true, detached: process.platform !== 'win32', shell: false,
    });
    let stdout = '', stderr = '', bytes = 0, stopped, settled = false;
    const finish = (error, code) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      signal?.removeEventListener('abort', abort);
      if (error || stopped) reject(error ?? new Error(stopped));
      else if (code !== 0) reject(new Error(`Deja-vu exited ${code}: ${stderr.slice(0, 240)}`));
      else resolveResult(stdout.trim());
    };
    const kill = (reason) => {
      if (stopped || settled) return;
      stopped = reason;
      if (!child.pid) return;
      if (process.platform === 'win32') {
        const killer = spawn('taskkill', ['/PID', String(child.pid), '/T', '/F'], { stdio: 'ignore', windowsHide: true });
        killer.once('error', () => child.kill());
        killer.once('exit', code => { if (code !== 0 && child.exitCode === null) child.kill(); });
      } else {
        try { process.kill(-child.pid, 'SIGKILL'); } catch (error) {
          if (error.code !== 'ESRCH') child.kill('SIGKILL');
        }
      }
    };
    const abort = () => kill('Deja-vu request cancelled');
    const timer = timeout > 0 ? setTimeout(() => kill('Deja-vu request timed out'), timeout) : undefined;
    signal?.addEventListener('abort', abort, { once: true });
    if (signal?.aborted) abort();
    for (const [stream, append] of [
      [child.stdout, text => { stdout += text; }],
      [child.stderr, text => { stderr += text; }],
    ]) {
      stream.setEncoding('utf8');
      stream.on('data', text => {
        bytes += Buffer.byteLength(text);
        if (bytes > maxBytes) kill('Deja-vu output exceeded budget');
        else append(text);
      });
    }
    child.once('error', error => finish(error));
    child.once('close', code => finish(undefined, code));
    child.stdin.on('error', error => { if (error.code !== 'EPIPE') kill('Deja-vu input failed'); });
    child.stdin.end(input);
  });
}

export class DejaClient {
  constructor(config, run = runProcess) {
    this.config = config;
    this.runProcess = run;
    this.owner = new AbortController();
    this.pending = new Set();
    this.warmup = undefined;
  }
  call(args, payload, ctx, timeout = 1500, maxBytes = 64 * 1024) {
    try {
      this.config.refresh?.();
      if (!this.config.executable) throw new Error('未找到外部 Deja，请输入 /deja 设置已安装的 Deja 程序；插件不内置引擎');
    } catch (error) { return Promise.reject(error); }
    const signals = [this.owner.signal, ...(ctx?.signal ? [ctx.signal] : [])];
    const request = this.runProcess(this.config.executable, args, {
      input: payload === undefined ? '' : JSON.stringify(payload), cwd: ctx?.cwd,
      env: { ...this.config.env, DEJA_NO_AUTO_INDEX: '1', DEJA_HOOK_REFRESH: '1' },
      signal: AbortSignal.any(signals), timeout, maxBytes,
    });
    this.pending.add(request);
    request.finally(() => this.pending.delete(request)).catch(() => {});
    return request;
  }
  async desktopRoot(ctx) {
    const result = JSON.parse(await this.call(['paths', 'show', 'pi-desktop', '--json'], undefined, ctx));
    const field = result.fields?.find(item => item.key === 'data');
    if (!field?.enabled || field.status !== 'available' || !isAbsolute(field.paths?.[0] ?? '')) {
      throw new Error('PI-Desktop source is disabled or unavailable; configure Agent paths in Deja-vu (P)');
    }
    return field.paths[0];
  }
  async memoryRevision() {
    this.config.refresh?.();
    const env = this.config.env;
    const paths = [this.config.configFile, env.DEJA_POLICY_FILE, env.DEJA_SOURCE_PATHS_FILE,
      env.XDG_CONFIG_HOME && join(env.XDG_CONFIG_HOME, 'deja', 'exclude')].filter(Boolean);
    return JSON.stringify([this.config.executable, env, await Promise.all(paths.map(async path => {
      try {
        const info = await stat(path, { bigint: true });
        return `${path}:${info.dev}:${info.ino}:${info.size}:${info.mtimeNs}:${info.ctimeNs}`;
      } catch (error) { if (error.code === 'ENOENT') return `${path}:missing`; throw error; }
    }))]);
  }
  // Not awaited by an event handler. All children still belong to this client
  // and are cancelled on disposal; hooks never spawn detached rebuilds.
  prepare(ctx, report) {
    if (this.warmup) return;
    this.warmup = (async () => {
      this.config.refresh?.();
      if (this.config.env.DEJA_PI_AUTO_INDEX === '0') return;
      const path = this.config.env.DEJA_POLICY_FILE;
      if (path) {
        let policy;
        try { policy = JSON.parse(await readFile(path, 'utf8')); }
        catch (error) { if (error.code !== 'ENOENT') throw error; }
        if (this.config.env.DEJA_PI_AUTO_INDEX !== '1' && policy?.auto_index === false) return;
      }
      await this.call(['index', '--quiet'], undefined, { cwd: ctx.cwd }, 120_000, 1024 * 1024);
    })()
      .catch(error => report(new Error(`Background index did not finish: ${error.message}`)))
      .finally(() => { this.warmup = undefined; });
  }
  async dispose() {
    this.owner.abort();
    await Promise.allSettled([...this.pending]);
  }
}
