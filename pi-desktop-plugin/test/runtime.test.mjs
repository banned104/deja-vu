import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, existsSync, mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { PassThrough } from 'node:stream';
import { configLocation, loadRuntime, readSettings, saveSettings, resolveRuntime, validateSettings } from '../src/runtime-config.mjs';
import { DejaClient, runProcess } from '../src/deja-client.mjs';
import { forward, serve } from '../src/mcp-bridge.mjs';
import { registerSettingsCommand, diagnostics } from '../src/settings-command.mjs';
import { stage } from '../scripts/stage.mjs';

function fixture(t) {
  const home=mkdtempSync(join(process.env.PI_SCRATCH_DIR??tmpdir(),'deja-runtime-'));
  t.after(()=>rmSync(home,{recursive:true,force:true}));
  const file=join(home,'config','pi-desktop.json');
  const options={file,home,environment:{HOME:home,USERPROFILE:home,PATH:''}};
  return {home,file,options};
}
test('unconfigured runtime is usable without installing, copying or indexing anything',t=>{
  const f=fixture(t), config=resolveRuntime(f.options);
  assert.equal(config.executable,'');assert.equal(config.settings.autoIndex,false);
  assert.equal(config.env.DEJA_PI_AUTO_INDEX,'0');assert.equal(config.env.DEJA_NO_AUTO_INDEX,'1');
  assert.equal(config.env.DEJA_INDEX_DIR,join(f.home,'.cache','deja','index.db'));
  assert.equal(existsSync(f.file),false);assert.equal(existsSync(config.env.DEJA_INDEX_DIR),false);
  assert.equal(configLocation({},f.home),join(f.home,'.config','deja','pi-desktop.json'));
});
test('persistent selection and shared paths refresh without changing package files',t=>{
  const f=fixture(t), config=loadRuntime(f.options);
  const index=join(f.home,'shared-index');
  saveSettings(f.file,{executable:process.execPath,environment:{DEJA_INDEX_DIR:index}});
  config.refresh();assert.equal(config.executable,process.execPath);assert.equal(config.env.DEJA_INDEX_DIR,index);
  saveSettings(f.file,{executable:join(f.home,'offline.exe')});
  config.refresh();assert.equal(config.executable,'');assert.equal(config.env.DEJA_INDEX_DIR,index);
  assert.equal(existsSync(index),false);
});
test('GUI and hooks ignore divergent ambient source/config secrets and use persistent settings',t=>{
  const f=fixture(t);
  const config=resolveRuntime({...f.options,environment:{...f.options.environment,DEJA_INDEX_DIR:join(f.home,'wrong'),DEJA_STORES:'claude',XDG_CONFIG_HOME:join(f.home,'wrong-config'),OPENAI_API_KEY:'fixture-secret'}});
  assert.equal(config.env.DEJA_INDEX_DIR,join(f.home,'.cache','deja','index.db'));
  assert.equal(config.env.DEJA_STORES,undefined);assert.equal(config.env.OPENAI_API_KEY,undefined);
  for(const key of ['DEJA_EMBED_KEY','DEJA_HERMES_PG_DSN','OPENAI_API_KEY','DEJA_UNKNOWN_ROOT']) assert.throws(()=>saveSettings(f.file,{environment:{[key]:'secret'}}),/不允许/);
  assert.equal(existsSync(f.file),false);
});
test('invalid, oversized and concurrently locked settings never silently fallback or overwrite',t=>{
  const f=fixture(t);
  for(const value of [{version:2},{version:1,autoIndex:'yes'},{version:1,executable:'relative'},{version:1,environment:{DEJA_INDEX_DIR:'relative'}}]) assert.throws(()=>validateSettings(value));
  mkdirSync(join(f.home,'config'));writeFileSync(f.file,'{broken');
  assert.throws(()=>resolveRuntime(f.options),/无法读取/);assert.match(loadRuntime(f.options).error,/无法读取/);
  assert.throws(()=>saveSettings(f.file,{autoIndex:true}));assert.equal(readFileSync(f.file,'utf8'),'{broken');
  writeFileSync(f.file,JSON.stringify({version:1}));writeFileSync(f.file+'.lock','other-owner');
  assert.throws(()=>saveSettings(f.file,{autoIndex:true}),/其他进程/);assert.equal(readSettings(f.file).autoIndex,false);
  assert.equal(readFileSync(f.file+'.lock','utf8'),'other-owner');
});
test('auto indexing is opt-in, honors policy, refreshes and refuses damaged policy',async t=>{
  const f=fixture(t);saveSettings(f.file,{executable:process.execPath});
  const config=loadRuntime(f.options),calls=[],errors=[];
  const client=new DejaClient(config,async(_exe,args)=>{calls.push(args);return ''});t.after(()=>client.dispose());
  client.prepare({cwd:f.home},e=>errors.push(e));await client.warmup;assert.equal(calls.length,0);
  saveSettings(f.file,{autoIndex:true});
  mkdirSync(join(f.home,'.config','deja'),{recursive:true});writeFileSync(config.env.DEJA_POLICY_FILE,'{"auto_index":false}');
  client.prepare({cwd:f.home},e=>errors.push(e));await client.warmup;assert.equal(calls.length,0);
  writeFileSync(config.env.DEJA_POLICY_FILE,'{}');client.prepare({cwd:f.home},e=>errors.push(e));await client.warmup;
  assert.deepEqual(calls,[['index','--quiet']]);
  writeFileSync(config.env.DEJA_POLICY_FILE,'{broken');client.prepare({cwd:f.home},e=>errors.push(e));await client.warmup;
  assert.equal(calls.length,1);assert.equal(errors.length,1);
});
test('memory revision changes with runtime settings and shared policy/path edits',async t=>{
  const f=fixture(t),client=new DejaClient(loadRuntime(f.options));t.after(()=>client.dispose());
  const before=await client.memoryRevision();saveSettings(f.file,{autoIndex:true});
  const after=await client.memoryRevision();assert.notEqual(after,before);
  mkdirSync(join(f.home,'.config','deja'),{recursive:true});writeFileSync(client.config.env.DEJA_SOURCE_PATHS_FILE,'{}');assert.notEqual(await client.memoryRevision(),after);
});
test('subprocess excludes provider credentials and ambient Deja overrides',async()=>{
  const before=process.env.OPENAI_API_KEY;process.env.OPENAI_API_KEY='never-inherit-fixture';
  try { const out=await runProcess(process.execPath,['-e','process.stdout.write(JSON.stringify({key:process.env.OPENAI_API_KEY,offline:process.env.DEJA_OFFLINE}))']);assert.deepEqual(JSON.parse(out),{offline:'1'}); }
  finally {if(before===undefined)delete process.env.OPENAI_API_KEY;else process.env.OPENAI_API_KEY=before;}
});
test('MCP forwards the original mode/project through external MCP without CLI reimplementation',async()=>{
  let seen;
  const args={mode:'remember',q:'synthetic decision',project:'explicit',cwd:process.cwd()};
  const result=await forward(args,{resolve:()=>({executable:'external-deja',env:{DEJA_INDEX_DIR:'shared'}}),run:async(exe,argv,options)=>{
    seen={exe,argv,options};return JSON.stringify({id:2,result:{content:[{type:'text',text:'remembered'}]}});
  }});
  assert.equal(seen.exe,'external-deja');assert.deepEqual(seen.argv,['mcp']);
  const messages=seen.options.input.trim().split('\n').map(line=>JSON.parse(line));
  const {cwd,...upstream}=args;assert.deepEqual(messages.at(-1).params.arguments,upstream);assert.equal(seen.options.cwd,cwd);assert.equal(seen.options.env.DEJA_INDEX_DIR,'shared');
  assert.equal(result.content[0].text,'remembered');
});
test('MCP handshake/list do not resolve or start Deja; missing engine is an actionable tool error',async()=>{
  const input=new PassThrough(),output=new PassThrough();let buffer='',lookups=0;
  const replies=new Map(),waiters=new Map();output.on('data',chunk=>{buffer+=chunk;let index;while((index=buffer.indexOf('\n'))>=0){const response=JSON.parse(buffer.slice(0,index));buffer=buffer.slice(index+1);replies.set(response.id,response);waiters.get(response.id)?.(response);}});
  const server=serve(input,output,{resolve:()=>{lookups++;return {executable:''}}});
  const request=message=>new Promise(resolve=>{waiters.set(message.id,resolve);input.write(JSON.stringify({jsonrpc:'2.0',...message})+'\n');});
  try {
    assert.ok((await request({id:1,method:'initialize'})).result);
    assert.equal((await request({id:2,method:'tools/list'})).result.tools[0].name,'deja');assert.equal(lookups,0);
    const result=await request({id:3,method:'tools/call',params:{name:'deja',arguments:{mode:'recall',q:'anything'}}});
    assert.equal(result.result.isError,true);assert.match(result.result.content[0].text,/\/deja/);assert.equal(lookups,1);
    input.write('{broken\n');await new Promise(resolve=>setImmediate(resolve));assert.equal(replies.get(null).error.code,-32700);
  } finally {server.close();input.destroy();output.destroy();}
});
test('MCP close cancels in-flight external requests',async()=>{
  const input=new PassThrough(),output=new PassThrough();let signal;
  const server=serve(input,output,{resolve:()=>({executable:'fixture',env:{}}),run:async(_exe,_args,options)=>{
    signal=options.signal;await new Promise((_resolve,reject)=>signal.addEventListener('abort',()=>reject(new Error('cancelled')),{once:true}));
  }});
  input.write(JSON.stringify({jsonrpc:'2.0',id:1,method:'tools/call',params:{name:'deja',arguments:{mode:'recall',cwd:process.cwd()}}})+'\n');
  await new Promise(resolve=>setImmediate(resolve));server.close();assert.equal(signal.aborted,true);
  await new Promise(resolve=>setImmediate(resolve));assert.equal(server.requests.size,0);input.destroy();output.destroy();
});
function commandStand(client, choices=[], confirmed=false) {
  let command;const messages=[],calls=[];
  registerSettingsCommand({registerCommand(name,value){assert.equal(name,'deja');command=value.handler;}},client);
  const ctx={cwd:process.cwd(),ui:{select:async()=>choices.shift(),input:async()=>choices.shift(),confirm:async()=>confirmed,notify:(...args)=>messages.push(args),setStatus(){}}};
  return {run:args=>command(args,ctx),messages,calls,ctx};
}
test('status entry works without Deja and never issues history/index calls',async t=>{
  const f=fixture(t);const calls=[],client=new DejaClient(loadRuntime(f.options),async(_exe,args)=>{calls.push(args);return ''});
  t.after(()=>client.dispose());const s=commandStand(client);await s.run('status');assert.match(s.messages[0][0],/未找到/);assert.match(s.messages[0][0],/不更新索引/);assert.equal(calls.length,0);
});
test('index menu requires explicit confirmation',async t=>{
  const f=fixture(t);const calls=[],client=new DejaClient(loadRuntime(f.options),async(_exe,args)=>{calls.push(args);return ''});t.after(()=>client.dispose());
  await commandStand(client,['更新索引（需确认）'],false).run('');assert.equal(calls.length,0);
});
test('staging includes source only, no EXE, personal settings or configure template',t=>{
  const f=fixture(t),output=join(f.home,'plugin');const manifest=stage(output);
  assert.equal(manifest.contributes.mcpServers[0].command,'node');
  assert.equal(manifest.contributes.mcpServers[0].args[0],'src/mcp-bridge.mjs');
  assert.deepEqual(manifest.contributes.mcpServers[0].env,{});
  const files=readdirSync(output,{recursive:true});assert.ok(files.includes('src'));
  assert.equal(files.some(file=>/\.(exe|dll)$/i.test(file)),false);assert.equal(existsSync(join(output,'bin')),false);
  assert.doesNotMatch(readFileSync(join(output,'manifest.json'),'utf8'),/UNCONFIGURED|USERPROFILE|build\/deja|C:\\\\Users/);
  assert.throws(()=>stage(output),/已存在/);
});

test('settings menu saves a verified external executable without copying it',async t=>{
  const f=fixture(t);let probes=0;
  class FakeClient extends DejaClient { constructor(config){super(config,async(_exe,args)=>{assert.deepEqual(args,['version']);probes++;return 'deja 0.23.0-pi-desktop.2';});} }
  const client=new FakeClient(loadRuntime(f.options));t.after(()=>client.dispose());
  const s=commandStand(client,['设置外部 Deja 程序',process.execPath],true);await s.run('');
  assert.equal(readSettings(f.file).executable,process.execPath);assert.equal(probes,1);
  assert.match(s.messages.at(-1)[0],/无需重打包/);assert.equal(existsSync(join(f.home,'bin')),false);
});

test('confirmed index menu executes one owned index request',async t=>{
  const f=fixture(t);saveSettings(f.file,{executable:process.execPath});const calls=[];
  const client=new DejaClient(loadRuntime(f.options),async(_exe,args)=>{calls.push(args);return ''});t.after(()=>client.dispose());
  const s=commandStand(client,['更新索引（需确认）'],true);await s.run('');
  assert.deepEqual(calls,[['index','--quiet']]);assert.match(s.messages.at(-1)[0],/更新完成/);
});

test('settings menu manages shared Agent paths without indexing',async t=>{
  const f=fixture(t);saveSettings(f.file,{executable:process.execPath});const calls=[];
  const field={agent:'pi-desktop',key:'data',kind:'directory',status:'available',paths:[f.home],enabled:true};
  const client=new DejaClient(loadRuntime(f.options),async(_exe,args)=>{calls.push(args);return args[1]==='list'?JSON.stringify({fields:[field]}):'Saved';});t.after(()=>client.dispose());
  const label='pi-desktop / data · directory · available';
  const s=commandStand(client,['管理 Agent 历史路径',label,'编辑路径',f.home],true);await s.run('');
  assert.deepEqual(calls,[['paths','list','--json'],['paths','set','pi-desktop','data',f.home]]);
  assert.match(s.messages.at(-1)[0],/索引尚未更新/);
});

test('failed executable verification never persists the selection',async t=>{
  const f=fixture(t);
  class FakeClient extends DejaClient {constructor(config){super(config,async()=> 'not deja');}}
  const client=new FakeClient(loadRuntime(f.options));t.after(()=>client.dispose());
  const s=commandStand(client,['设置外部 Deja 程序',process.execPath],true);await s.run('');
  assert.equal(existsSync(f.file),false);assert.match(s.messages.at(-1)[0],/未返回 Deja 版本/);
});

test('MCP refuses missing or relative working directories before starting an external process',async()=>{
  let calls=0;const options={resolve:()=>({executable:'fixture',env:{}}),run:async()=>{calls++;return '';}};
  await assert.rejects(forward({mode:'remember',q:'must not misfile'},options),/cwd/);
  await assert.rejects(forward({mode:'recall',cwd:'.'},options),/cwd/);assert.equal(calls,0);
});

test('diagnostics retains a stale selected executable path',async t=>{
  const f=fixture(t),stale=join(f.home,'moved-deja.exe');saveSettings(f.file,{executable:stale});
  const client=new DejaClient(loadRuntime(f.options));t.after(()=>client.dispose());
  const text=await diagnostics(client,{});assert.match(text,/已保存但不可执行/);assert.ok(text.includes(stale));
});

test('manual no-time-limit requests remain cancellable',async()=>{
  const controller=new AbortController();
  const pending=runProcess(process.execPath,['-e','setInterval(()=>{},1000)'],{timeout:0,signal:controller.signal});
  controller.abort();await assert.rejects(pending,/cancelled/);
});

test('relative source edits are rejected before paths set',async t=>{
  const f=fixture(t);saveSettings(f.file,{executable:process.execPath});const calls=[];
  const field={agent:'pi-desktop',key:'data',kind:'directory',status:'available',paths:[f.home],enabled:true};
  const client=new DejaClient(loadRuntime(f.options),async(_exe,args)=>{calls.push(args);return JSON.stringify({fields:[field]});});t.after(()=>client.dispose());
  const s=commandStand(client,['管理 Agent 历史路径','pi-desktop / data · directory · available','编辑路径','./sessions'],true);
  await s.run('');assert.equal(calls.length,1);assert.match(s.messages.at(-1)[0],/必须是绝对路径/);
});

test('manual index uses cancellation ownership without a fixed wall-clock timeout',async t=>{
  const f=fixture(t);saveSettings(f.file,{executable:process.execPath});let timeout;
  const client=new DejaClient(loadRuntime(f.options),async(_exe,_args,options)=>{timeout=options.timeout;return '';});t.after(()=>client.dispose());
  await commandStand(client,['更新索引（需确认）'],true).run('');assert.equal(timeout,0);
});
