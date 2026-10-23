import { readFileSync, statSync } from 'node:fs';
import { isAbsolute, join } from 'node:path';
import { saveSettings, readSettings, findExecutable } from './runtime-config.mjs';

const limit = text => String(text).slice(0, 12_000);
function indexStatus(path) {
  try { const info = statSync(join(path, 'manifest.gob')); return `已有索引，元数据更新时间 ${info.mtime.toISOString()}`; }
  catch { return '尚无可用索引元数据；需要明确执行更新索引'; }
}
export async function diagnostics(client, ctx) {
  client.config.refresh?.();
  const config = client.config, env = config.env;
  const lines = [
    `外部 Deja：${config.executable || (config.selectedExecutable ? `已保存但不可执行：${config.selectedExecutable}` : '未找到；请设置已安装的程序')}（插件不带 EXE）`,
    `插件配置：${config.configFile}`,
    `索引：${env.DEJA_INDEX_DIR}`, indexStatus(env.DEJA_INDEX_DIR),
    `笔记：${env.DEJA_NOTES_FILE}`,
    `策略：${env.DEJA_POLICY_FILE}`,
    `Agent 路径配置：${env.DEJA_SOURCE_PATHS_FILE}`,
    `数据源范围：${env.DEJA_STORES || 'Deja 常规发现；受用户禁用/隐私规则约束'}`,
    `插件自动索引：${config.settings.autoIndex ? '已允许；仍受共享策略限制' : '关闭（默认）'}`,
    `sqlite3：${config.sqliteExecutable || '未找到；数据库数据源需要 sqlite3'}`,
    'MCP：桥由 PI 宿主管理；每次调用读取本配置并连接外部 Deja，无需重打包。',
  ];
  if (config.executable) {
    try {
      lines.push(`版本：${await client.call(['version'], undefined, ctx, 3000)}`);
      const report = JSON.parse(await client.call(['paths', 'show', 'pi-desktop', '--json'], undefined, ctx, 3000));
      lines.push(...(report.fields ?? []).map(field => `PI-Desktop：${field.paths.join(' | ')}；${field.origin}；${field.status}；${field.enabled ? '启用' : '禁用'}`));
    } catch (error) { lines.push(`兼容性/路径检查失败：${error.message}。完整 Desktop 能力需要支持 paths 和 pi-desktop 的 Deja 版本。`); }
  }
  try { const policy = JSON.parse(readFileSync(env.DEJA_POLICY_FILE, 'utf8')); if (policy.auto_index === false) lines.push('共享 policy 禁止自动索引。'); }
  catch (error) { if (error.code !== 'ENOENT') lines.push(`策略不可读/损坏：${error.message}；不执行自动索引。`); }
  lines.push('本检查不解析会话正文、不更新索引、不调用模型。');
  return limit(lines.join('\n'));
}
export function registerSettingsCommand(pi, client) {
  pi.registerCommand('deja', {
    description: 'Deja-vu：配置外部程序、检查状态、管理 Agent 历史路径和更新索引',
    handler: async (args, ctx) => {
      const notify = (text, kind = 'info') => ctx.ui.notify(limit(text), kind);
      try {
        const action = String(args ?? '').trim() === 'status' ? '查看状态（不索引）' : await ctx.ui.select('Deja-vu · 共用本机 Deja', [
          '查看状态（不索引）', '设置外部 Deja 程序', '管理 Agent 历史路径', '设置共享数据位置',
          '更新索引（需确认）', '设置自动索引（默认关闭）',
        ]);
        if (!action) return;
        if (action === '查看状态（不索引）') return notify(await diagnostics(client, ctx));
        client.config.refresh?.();
        const config = client.config;
        if (action === '设置外部 Deja 程序') {
          const value = await ctx.ui.input('已安装 Deja 的绝对路径；输入 auto 恢复自动查找', config.selectedExecutable || config.executable || '例如 D:\\Tools\\deja\\deja.exe');
          if (value === undefined || !value.trim()) return;
          const executable = value.trim() === 'auto' ? '' : value.trim();
          if (executable) {
            let regular = false; try { regular = isAbsolute(executable) && statSync(executable).isFile(); } catch {}
            if (!regular || (process.platform === 'win32' && !executable.toLowerCase().endsWith('.exe'))) throw new Error(`Deja 程序不可用，请选择已安装原生程序的绝对路径：${executable}`);
          }
          if (!await ctx.ui.confirm('执行程序检查', `将执行你选择的程序 version，仅用于验证版本。\n${executable || '自动定位的 deja'}`)) return;
          const probe = new client.constructor({ executable: executable || findExecutable('deja'), env: config.env });
          try { const version = await probe.call(['version'], undefined, ctx, 3000); if (!/^deja\s+\S+/i.test(version)) throw new Error('该程序未返回 Deja 版本'); }
          finally { await probe.dispose(); }
          saveSettings(config.configFile, { executable });
          config.refresh?.();
          return notify('已保存。MCP 与自动 hooks 下一次调用共用该程序，无需重打包。');
        }
        if (action === '设置共享数据位置') {
          const keys = ['DEJA_INDEX_DIR', 'DEJA_NOTES_FILE', 'DEJA_POLICY_FILE', 'DEJA_SOURCE_PATHS_FILE', 'XDG_CONFIG_HOME'];
          const key = await ctx.ui.select('共享数据位置；默认与独立 Deja 相同', keys);
          if (!key) return;
          const value = await ctx.ui.input(`${key}：绝对路径；auto 恢复默认`, config.env[key]);
          if (!value?.trim()) return;
          const environment = { ...readSettings(config.configFile).environment };
          if (value.trim() === 'auto') delete environment[key]; else environment[key] = value.trim();
          if (!await ctx.ui.confirm('更改共享数据位置', '只保存路径，不迁移、不合并、不删除数据、不重建索引。请确认这里与独立 Deja 使用的位置相同。')) return;
          saveSettings(config.configFile, { environment });
          return notify('已保存。MCP/hooks 下一次调用生效；独立 CLI 请使用相同路径或相同环境覆盖。');
        }
        if (action === '设置自动索引（默认关闭）') {
          const enabled = await ctx.ui.select('自动索引', ['关闭', '开启（会读取已启用的真实历史）']);
          if (!enabled) return;
          if (enabled.startsWith('开启') && !await ctx.ui.confirm('允许自动索引', '会话开始和 Agent 结束时将更新共享索引，读取当前启用范围内的真实历史。共享策略仍然生效。是否允许？')) return;
          saveSettings(config.configFile, { autoIndex: enabled.startsWith('开启') });
          return notify('自动索引设置已保存；保存本身不执行索引。');
        }
        if (action === '更新索引（需确认）') {
          if (!await ctx.ui.confirm('更新共享 Deja 索引', `将读取所有已启用数据源的真实历史并更新：\n${config.env.DEJA_INDEX_DIR}\n不删除源会话。继续？`)) return;
          ctx.ui.setStatus?.('deja', 'Deja-vu 正在更新共享索引…');
          try { const output = await client.call(['index', '--quiet'], undefined, ctx, 0, 1024 * 1024); notify(`索引更新完成。${output ? `\n${output}` : ''}`); }
          finally { ctx.ui.setStatus?.('deja', ''); }
          return;
        }
        if (action === '管理 Agent 历史路径') {
          const report = JSON.parse(await client.call(['paths', 'list', '--json'], undefined, ctx, 5000));
          const fields = report.fields ?? [];
          const labels = fields.map(field => `${field.agent} / ${field.key} · ${field.kind} · ${field.status}`);
          const selected = await ctx.ui.select('Agent 历史路径（保存不更新索引）', labels);
          if (!selected) return;
          const field = fields[labels.indexOf(selected)];
          const choice = await ctx.ui.select(`${field.agent}/${field.key}：${field.paths.join(' | ')}`, ['编辑路径', '检查源文件（不解析正文）', field.enabled ? '禁用整个 Agent' : '启用整个 Agent']);
          if (!choice) return;
          if (choice === '检查源文件（不解析正文）') return notify(await client.call(['paths', 'check', field.agent], undefined, ctx, 10_000));
          if (choice === '编辑路径') {
            const input = await ctx.ui.input('绝对路径；多目录字段用换行分隔', field.paths.join('\n'));
            if (!input?.trim()) return;
            const paths = input.split(/\r?\n/).map(path => path.trim()).filter(Boolean);
            if (!paths.every(isAbsolute)) throw new Error('每个历史路径都必须是绝对路径；尚未保存');
            await client.call(['paths', 'set', field.agent, field.key, ...paths], undefined, ctx, 5000);
          } else {
            if (!await ctx.ui.confirm(choice, '修改整个 Agent 的启用状态；不删除原始历史，不执行索引。继续？')) return;
            await client.call(['paths', field.enabled ? 'disable' : 'enable', field.agent], undefined, ctx, 5000);
          }
          return notify('已保存共享 Agent 路径设置；CLI、MCP 和 hooks 使用相同文件。索引尚未更新。');
        }
      } catch (error) { notify(`Deja-vu：${error.message}\n插件不会阻断聊天。`, 'error'); }
    },
  });
}
