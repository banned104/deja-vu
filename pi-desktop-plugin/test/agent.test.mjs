import test from 'node:test';
import assert from 'node:assert/strict';
import { attach, bounded, historical } from '../src/agent.mjs';

function stand() {
  const handlers = new Map(), calls = [], status = [];
  const client = {
    config: { env: {} },
    async call(args, payload) {
      calls.push({ args, payload });
      if (args[0] === 'hook-context') return JSON.stringify({ hookSpecificOutput: { additionalContext: `decision for ${payload.cwd}` } });
      if (args[0] === 'hook-prompt') return '';
      return 'Historical fix: use a version key.';
    }, prepare() {}, async dispose() {},
  };
  const pi = { on(name, handler) { handlers.set(name, handler); } };
  const state = attach(pi, client);
  const ctx = (id = 'session-a', cwd = '/project/a', signal) => ({
    cwd, signal, sessionManager: { getSessionId: () => id },
    ui: { setStatus: (...args) => status.push(args) },
  });
  const emit = (name, event, context = ctx()) => handlers.get(name)(event, context);
  return { client, calls, status, state, ctx, emit };
}

test('recall reaches the provider prompt and preserves previous additions across turns', async () => {
  const s = stand();
  const first = await s.emit('before_agent_start', { prompt: 'cache', systemPrompt: 'CORE + PEER' });
  assert.ok(first.systemPrompt.startsWith('CORE + PEER'));
  assert.match(first.systemPrompt, /decision for \/project\/a/);
  assert.match(first.systemPrompt, /untrusted data/);
  const second = await s.emit('before_agent_start', { prompt: 'next', systemPrompt: 'FRESH CORE' });
  assert.match(second.systemPrompt, /^FRESH CORE/);
  assert.match(second.systemPrompt, /decision for/);
  assert.equal(s.calls.filter(c => c.args[0] === 'hook-context').length, 1);
});

test('concurrent sessions and project changes do not share recalled state', async () => {
  const s = stand();
  const [a,b] = await Promise.all([
    s.emit('before_agent_start', {prompt:'a',systemPrompt:'A'},s.ctx('a','/a')),
    s.emit('before_agent_start', {prompt:'b',systemPrompt:'B'},s.ctx('b','/b')),
  ]);
  assert.match(a.systemPrompt,/decision for \/a/); assert.doesNotMatch(a.systemPrompt,/decision for \/b/);
  assert.match(b.systemPrompt,/decision for \/b/);
  const c = await s.emit('before_agent_start',{prompt:'c',systemPrompt:'C'},s.ctx('a','/changed'));
  assert.match(c.systemPrompt,/decision for \/changed/);
  assert.deepEqual(s.calls.filter(c=>c.args[0]==='hook-context').map(c=>c.payload.session_id),['a','b','a']);
});

test('tool hints retain original content and error metadata and are deduplicated', async () => {
  const s = stand();
  const original = {toolName:'Bash',toolCallId:'t1',input:{command:'bad'},content:[{type:'text',text:'failed'}],isError:true,details:{exitCode:1}};
  const copy = structuredClone(original);
  const result = await s.emit('tool_result',original);
  assert.deepEqual(original,copy);
  assert.deepEqual(result.content[0],original.content[0]);
  assert.equal(result.isError,undefined); assert.equal(result.details,undefined);
  assert.equal((await s.emit('tool_result',original)),undefined);
  assert.equal(s.calls[0].args[0],'hook-tool-after');
  await s.emit('tool_result',{toolName:'Read',toolCallId:'r',input:{file_path:'src/a.ts'},content:[]});
  assert.equal(s.calls.at(-1).payload.tool_input.file_path,'src/a.ts');
});

test('compaction invalidates cached recall and missing binary fails open', async () => {
  const s = stand();
  await s.emit('before_agent_start',{prompt:'a',systemPrompt:'base'});
  await s.emit('session_compact',{});
  await s.emit('before_agent_start',{prompt:'b',systemPrompt:'base'});
  assert.equal(s.calls.filter(c=>c.args[0]==='hook-context').length,2);
  s.client.call = async()=>{throw new Error('ENOENT fixture')};
  assert.equal(await s.emit('before_agent_start',{prompt:'c',systemPrompt:'base'}),undefined);
  assert.ok(s.status.some(args=>String(args[1]).includes('/deja')));
});

test('late cancelled results are never injected', async () => {
  const s=stand(), controller=new AbortController();
  s.client.call=async()=>{controller.abort();return 'late result'};
  assert.equal(await s.emit('before_agent_start',{prompt:'a',systemPrompt:'base'},s.ctx('a','/a',controller.signal)),undefined);
});

test('Unicode and hostile quoted evidence stay within output budgets',()=>{
  assert.equal(bounded('中😀abc',7),'中😀');
  const result=historical('"\\\n'.repeat(10000),1800);
  assert.ok(Buffer.byteLength(result)<=1800);
  assert.equal(historical(''), '');
});

test('native pi sessions are ignored rather than indexed as Desktop sessions',async()=>{
  const s=stand(),ctx=s.ctx();
  ctx.sessionManager.getSessionFile=()=>'/native/session.jsonl';
  await s.emit('session_start',{},ctx);
  assert.equal(await s.emit('before_agent_start',{prompt:'a',systemPrompt:'base'},ctx),undefined);
  assert.equal(s.calls.length,0);
});

test('disposed runner is inert and a fresh runner receives recall',async()=>{
  const old=stand();let disposed=false;
  old.client.dispose=async()=>{disposed=true};
  // Native guard avoids launching a real final child in this unit test.
  const ctx=old.ctx();ctx.sessionManager.getSessionFile=()=>'/native/session.jsonl';
  await old.emit('session_shutdown',{},ctx);
  assert.equal(disposed,true);assert.equal(old.state.sessions.size,0);
  assert.equal(await old.emit('before_agent_start',{prompt:'a',systemPrompt:'base'}),undefined);
  const fresh=stand();
  assert.match((await fresh.emit('before_agent_start',{prompt:'a',systemPrompt:'base'})).systemPrompt,/decision for/);
});

test('shared configuration revision revokes cached recall in the same session', async () => {
  const s=stand();let revision='before';
  s.client.memoryRevision=async()=>revision;
  await s.emit('before_agent_start',{prompt:'a',systemPrompt:'BASE'});
  await s.emit('before_agent_start',{prompt:'b',systemPrompt:'BASE'});
  assert.equal(s.calls.filter(c=>c.args[0]==='hook-context').length,1);
  revision='after';s.client.call=async()=>'';
  assert.equal(await s.emit('before_agent_start',{prompt:'c',systemPrompt:'BASE'}),undefined);
});
