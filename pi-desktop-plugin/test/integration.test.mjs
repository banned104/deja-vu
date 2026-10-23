import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, readFileSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { spawnSync } from 'node:child_process';
import { runProcess, DejaClient, loadConfig as loadRuntime } from '../src/deja-client.mjs';
import { attach } from '../src/agent.mjs';
import { stage } from '../scripts/stage.mjs';
import { saveSettings, sharedKeys } from '../src/runtime-config.mjs';
import { forward } from '../src/mcp-bridge.mjs';

const roots = new Map();
function configure({binary, output, environment, home}) {
  const manifest=stage(output), file=output+'.user.json';
  saveSettings(file,{executable:binary,environment:Object.fromEntries(Object.entries(environment).filter(([key,value])=>sharedKeys.has(key)&&value!==''))});
  roots.set(output,{file,home,environment:{...process.env,HOME:home,USERPROFILE:home}});
  return manifest;
}
function loadConfig(root) { return loadRuntime(roots.get(root)); }

const binary=process.env.DEJA_TEST_BINARY;
function fixture(t) {
  const root=mkdtempSync(join(process.env.PI_SCRATCH_DIR ?? tmpdir(),'deja-desktop-test-'));
  const cleanup=[];
  t.after(async()=>{for(const close of cleanup)await close();rmSync(root,{recursive:true,force:true});});
  const data=join(root,'desktop'), cwd=join(root,'project','cache');
  mkdirSync(join(data,'sessions'),{recursive:true});mkdirSync(cwd,{recursive:true});
  const sql=query=>{
    const result=spawnSync('sqlite3',[join(data,'pi.sqlite')],{input:query,encoding:'utf8'});
    assert.equal(result.status,0,result.stderr);
  };
  sql(`CREATE TABLE projects(id INTEGER PRIMARY KEY,path TEXT); CREATE TABLE sessions(id TEXT PRIMARY KEY,title TEXT,project_id INTEGER,deleted_at INTEGER); INSERT INTO projects VALUES(1,'${cwd.replaceAll("'","''")}'); INSERT INTO sessions VALUES('desktop-demo','Cache decision',1,NULL);`);
  const source=readFileSync(new URL('../../fixtures/registry/pi-desktop/desktop-demo.jsonl',import.meta.url),'utf8');
  const path=join(data,'sessions','desktop-demo.jsonl');writeFileSync(path,source);
  const settingsFile=join(root,'shared-source-paths.json');
  writeFileSync(settingsFile,JSON.stringify({version:1,agents:{'pi-desktop':{paths:{data:[data]}}}}));
  const env={HOME:root,USERPROFILE:root,XDG_CONFIG_HOME:join(root,'config'),XDG_DATA_HOME:join(root,'data'),
    DEJA_STORES:'pi-desktop,notes',DEJA_PI_DESKTOP_ROOT:'',PI_DESKTOP_DATA_DIR:'',DEJA_SOURCE_PATHS_FILE:settingsFile,DEJA_INDEX_DIR:join(root,'index.db'),
    DEJA_NOTES_FILE:join(root,'notes.jsonl'),DEJA_OFFLINE:'1',DEJA_NO_AUTO_INDEX:'1',DEJA_HOOK_REFRESH:'1'};
  const cli=(args,input='')=>runProcess(binary,args,{input,cwd,env,timeout:30000,maxBytes:1024*1024});
  return {root,data,cwd,sql,path,source,env,cli,cleanup};
}

test('real index refreshes metadata, rewritten speech, and soft deletion', {skip:!binary}, async t=>{
  const f=fixture(t);
  await f.cli(['index']);
  const first=await f.cli(['search','quartz-token','--json']);
  assert.match(first,/pi-desktop/);assert.match(first,/desktop-demo/);
  await f.cli(['index']);
  const second=JSON.parse(await f.cli(['search','quartz-token','--json']));
  const initial=JSON.parse(first);
  assert.equal(second.total,initial.total);
  assert.deepEqual(second.hits.map(h=>h.session),initial.hits.map(h=>h.session));
  f.sql("UPDATE sessions SET title='Renamed epoch decision'; UPDATE projects SET path='/work/changed-project';");
  await f.cli(['index']);
  const renamed=await f.cli(['search','quartz-token','--json']);
  assert.match(renamed,/Renamed epoch decision/);assert.match(renamed,/changed-project/);
  writeFileSync(f.path,f.source.replaceAll('quartz-token','sapphire-token'));
  await f.cli(['index']);
  assert.doesNotMatch(await f.cli(['search','quartz-token','--json']),/desktop-demo/);
  assert.match(await f.cli(['search','sapphire-token','--json']),/desktop-demo/);
  f.sql('UPDATE sessions SET deleted_at=42;');await f.cli(['index']);
  assert.doesNotMatch(await f.cli(['search','sapphire-token','--json']),/desktop-demo/);
});

test('MCP handshake, recall, and explicit project remember use the isolated index', {skip:!binary},async t=>{
  const f=fixture(t);await f.cli(['index']);
  const messages=[
    {jsonrpc:'2.0',id:1,method:'initialize',params:{protocolVersion:'2024-11-05',capabilities:{},clientInfo:{name:'isolated-test',version:'1'}}},
    {jsonrpc:'2.0',method:'notifications/initialized'},
    {jsonrpc:'2.0',id:2,method:'tools/list'},
    {jsonrpc:'2.0',id:3,method:'tools/call',params:{name:'deja',arguments:{mode:'recall',q:'quartz-token'}}},
    {jsonrpc:'2.0',id:4,method:'tools/call',params:{name:'deja',arguments:{mode:'remember',q:'Synthetic amber-key decision: use epochs.',project:'explicit-project'}}},
  ];
  const raw=await f.cli(['mcp'],messages.map(x=>JSON.stringify(x)).join('\n')+'\n');
  const replies=raw.split('\n').map(x=>JSON.parse(x));
  assert.ok(replies.find(r=>r.id===1)?.result);
  assert.equal(replies.find(r=>r.id===2)?.result.tools[0].name,'deja');
  assert.match(JSON.stringify(replies.find(r=>r.id===3)),/quartz-token/);
  assert.doesNotMatch(JSON.stringify(replies.find(r=>r.id===4)),/"isError":true/);
  const notes=readFileSync(f.env.DEJA_NOTES_FILE,'utf8');
  assert.match(notes,/explicit-project/);assert.match(notes,/amber-key/);
});

test('configured plugin loads and actual hook evidence reaches provider-bound system prompt', {skip:!binary},async t=>{
  const f=fixture(t);
  const output=join(f.root,'plugin');
  configure({binary:resolve(binary),indexDir:f.env.DEJA_INDEX_DIR,output,sourcePathsFile:f.env.DEJA_SOURCE_PATHS_FILE,environment:f.env,home:f.root});
  const config=loadConfig(output);
  assert.equal(config.env.DEJA_SOURCE_PATHS_FILE,f.env.DEJA_SOURCE_PATHS_FILE);
  assert.equal(config.env.DEJA_PI_DESKTOP_ROOT,undefined);
  assert.equal(config.env.DEJA_INDEX_DIR,f.env.DEJA_INDEX_DIR);
  await f.cli(['index']);
  const client=new DejaClient({...config,env:{...config.env,HOME:f.root,USERPROFILE:f.root}});
  t.after(()=>client.dispose());
  const handlers=new Map();attach({on:(name,handler)=>handlers.set(name,handler)},client);
  const ctx={cwd:f.cwd,sessionManager:{getSessionId:()=> 'fresh-reader'},ui:{setStatus(){}}};
  const start=performance.now();
  const result=await handlers.get('before_agent_start')({prompt:'quartz-token cache',systemPrompt:'PROVIDER BASE'},ctx);
  assert.ok(result?.systemPrompt.startsWith('PROVIDER BASE'));
  assert.match(result.systemPrompt,/quartz-token/);
  t.diagnostic(`warm hook elapsed ${Math.round(performance.now()-start)} ms; injected ${Buffer.byteLength(result.systemPrompt)-13} bytes`);
  // Live-stamp protection: a source cannot recall its own transcript.
  const ownRaw=await f.cli(['hook-context'],JSON.stringify({session_id:'desktop-demo',cwd:f.cwd}));
  const own=ownRaw ? JSON.parse(ownRaw) : {};
  assert.doesNotMatch(own.hookSpecificOutput?.additionalContext ?? '',/Decided to invalidate quartz-token/);
});

test('compaction captures persisted Desktop state and returns it after compaction', {skip:!binary},async t=>{
  const f=fixture(t);await f.cli(['index']);
  const input=JSON.stringify({session_id:'desktop-demo',harness:'pi-desktop',cwd:f.cwd});
  await f.cli(['hook-precompact','--harness','pi-desktop'],input);
  const raw=await f.cli(['hook-context'],input);
  assert.match(raw,/quartz-token|cacheEpoch/);
  assert.match(raw,/compact|continu|recover|working|checkpoint/i);
});

test('active WAL metadata changes are visible without a transcript write', {skip:!binary},async t=>{
  const f=fixture(t);
  const {DatabaseSync}=await import('node:sqlite');
  const db=new DatabaseSync(join(f.data,'pi.sqlite'));
  try {
    db.exec('PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=0;');
    await f.cli(['index']);
    db.exec("UPDATE sessions SET title='WAL-only metadata title';");
    await f.cli(['index']);
    assert.match(await f.cli(['search','quartz-token','--json']),/WAL-only metadata title/);
    db.exec('UPDATE sessions SET deleted_at=99;');
    await f.cli(['index']);
    assert.doesNotMatch(await f.cli(['search','quartz-token','--json']),/desktop-demo/);
  } finally {db.close();}
});

test('Desktop speech is redacted by the existing index writer', {skip:!binary},async t=>{
  const f=fixture(t);
  const secret='sk-proj-'+ 'A7b9C2d4E6f8'.repeat(8);
  const message={type:'message',id:'secret-test',role:'assistant',createdAt:'2026-10-01T09:02:00Z',blocks:[{type:'text',text:`Synthetic quartz-token credential api_key=${secret}`} ]};
  writeFileSync(f.path,f.source+JSON.stringify(message)+'\n');
  await f.cli(['index']);
  const result=await f.cli(['show','desktop-demo','--json']);
  assert.doesNotMatch(result,new RegExp(secret));
  assert.match(result,/redacted/);
});

test('configured manifest has one server, minimal grants and refuses overwrite', {skip:!binary},t=>{
  const f=fixture(t),output=join(f.root,'configured');
  const options={binary:resolve(binary),indexDir:f.env.DEJA_INDEX_DIR,output,sourcePathsFile:f.env.DEJA_SOURCE_PATHS_FILE,environment:f.env,home:f.root};
  const manifest=configure(options);
  assert.equal(manifest.contributes.mcpServers.length,1);
  assert.equal(manifest.contributes.mcpServers[0].id,'deja');
  assert.deepEqual(manifest.permissions,['agent.extension','mcp.server.local']);
  assert.throws(()=>configure(options),/已存在/);
});

test('actual hooks keep two project histories separate', {skip:!binary},async t=>{
  const f=fixture(t),other=join(f.root,'other','billing');mkdirSync(other,{recursive:true});
  f.sql(`INSERT INTO projects VALUES(2,'${other.replaceAll("'","''")}'); INSERT INTO sessions VALUES('billing-demo','Billing decision',2,NULL);`);
  writeFileSync(join(f.data,'sessions','billing-demo.jsonl'),f.source.replaceAll('desktop-demo','billing-demo').replaceAll('quartz-token','cobalt-invoice'));
  await f.cli(['index']);
  const client=new DejaClient({executable:binary,env:f.env});t.after(()=>client.dispose());
  const handlers=new Map();attach({on:(name,handler)=>handlers.set(name,handler)},client);
  const context=(id,cwd)=>({cwd,sessionManager:{getSessionId:()=>id},ui:{setStatus(){}}});
  const [cache,billing]=await Promise.all([
    handlers.get('before_agent_start')({prompt:'quartz-token cache',systemPrompt:'CACHE BASE'},context('cache-reader',f.cwd)),
    handlers.get('before_agent_start')({prompt:'cobalt-invoice cache',systemPrompt:'BILLING BASE'},context('billing-reader',other)),
  ]);
  assert.match(cache.systemPrompt,/quartz-token/);assert.doesNotMatch(cache.systemPrompt,/cobalt-invoice/);
  assert.match(billing.systemPrompt,/cobalt-invoice/);assert.doesNotMatch(billing.systemPrompt,/quartz-token/);
});

test('Desktop shutdown retires requests and releases its live stamp', {skip:!binary},async t=>{
  const f=fixture(t);await f.cli(['index']);
  const client=new DejaClient({executable:binary,env:f.env});
  const handlers=new Map();const runtime=attach({on:(name,handler)=>handlers.set(name,handler)},client);
  const ctx={cwd:f.cwd,sessionManager:{getSessionId:()=> 'shutdown-reader'},ui:{setStatus(){}}};
  await handlers.get('before_agent_start')({prompt:'quartz-token',systemPrompt:'BASE'},ctx);
  await handlers.get('session_shutdown')({},ctx);
  assert.equal(client.owner.signal.aborted,true);assert.equal(client.pending.size,0);assert.equal(runtime.sessions.size,0);
  assert.equal(await handlers.get('before_agent_start')({prompt:'a',systemPrompt:'BASE'},ctx),undefined);
});

test('terminal path changes are picked up by plugin without reinstalling', {skip:!binary},async t=>{
  const f=fixture(t);
  const output=join(f.root,'shared-plugin');
  configure({binary:resolve(binary),indexDir:f.env.DEJA_INDEX_DIR,output,sourcePathsFile:f.env.DEJA_SOURCE_PATHS_FILE,environment:f.env,home:f.root});
  const config=loadConfig(output);
  const client=new DejaClient({...config,env:{...config.env,HOME:f.root,USERPROFILE:f.root,PI_DESKTOP_DATA_DIR:''}});
  t.after(()=>client.dispose());
  assert.equal(await client.desktopRoot({cwd:f.cwd}),f.data);
  const moved=join(f.root,'moved-desktop');
  mkdirSync(join(moved,'sessions'),{recursive:true});
  const {copyFileSync}=await import('node:fs');copyFileSync(join(f.data,'pi.sqlite'),join(moved,'pi.sqlite'));
  await f.cli(['paths','set','pi-desktop','data',moved]);
  assert.equal(await client.desktopRoot({cwd:f.cwd}),moved);
  await f.cli(['paths','disable','pi-desktop']);
  await assert.rejects(client.desktopRoot({cwd:f.cwd}),/disabled or unavailable/);
});

test('direct binary discovers default Desktop history without plugin root environment', {skip:!binary},async t=>{
  const f=fixture(t);const {renameSync}=await import('node:fs');
  const defaultRoot=join(f.root,'.pi-desktop');renameSync(f.data,defaultRoot);
  const env={...f.env,DEJA_SOURCE_PATHS_FILE:join(f.root,'absent-path-settings.json'),APPDATA:join(f.root,'appdata')};
  await runProcess(binary,['index'],{cwd:f.cwd,env,timeout:30000});
  const report=JSON.parse(await runProcess(binary,['paths','show','pi-desktop','--json'],{cwd:f.cwd,env}));
  assert.equal(report.fields[0].paths[0],defaultRoot);
  const result=await runProcess(binary,['search','quartz-token','--json'],{cwd:f.cwd,env});
  assert.match(result,/pi-desktop/);assert.match(result,/desktop-demo/);
});

test('unavailable saved root refuses an update and preserves the previous index', {skip:!binary},async t=>{
  const f=fixture(t);await f.cli(['index']);
  await f.cli(['paths','set','pi-desktop','data',join(f.root,'offline-drive')]);
  await assert.rejects(f.cli(['index']),/unavailable/);
  assert.match(await f.cli(['search','quartz-token','--json']),/desktop-demo/);
});

async function mcp(client, cwd, mode, q, project) {
  const messages = [
    {jsonrpc:'2.0',id:1,method:'initialize',params:{protocolVersion:'2024-11-05',capabilities:{},clientInfo:{name:'shared-memory-test',version:'1'}}},
    {jsonrpc:'2.0',method:'notifications/initialized'},
    {jsonrpc:'2.0',id:2,method:'tools/call',params:{name:'deja',arguments:{mode,q,...(project?{project}:{})}}},
  ];
  return client.runProcess(client.config.executable,['mcp'],{
    input:messages.map(message=>JSON.stringify(message)).join('\n')+'\n',cwd,
    env:client.config.env,timeout:30000,maxBytes:1024*1024,
  });
}

test('configured plugin and standalone reuse one index and bidirectional notes', {skip:!binary},async t=>{
  const f=fixture(t);
  await f.cli(['remember','Synthetic shared-jade note from terminal','--project','explicit-project']);
  await f.cli(['index']);
  const output=join(f.root,'shared-memory-plugin');
  configure({binary:resolve(binary),output,environment:f.env,home:f.root});
  const config=loadConfig(output);
  assert.equal(config.env.DEJA_INDEX_DIR,f.env.DEJA_INDEX_DIR);
  assert.equal(config.env.DEJA_NOTES_FILE,f.env.DEJA_NOTES_FILE);
  assert.equal(config.env.XDG_CONFIG_HOME,f.env.XDG_CONFIG_HOME);
  const client=new DejaClient(config);t.after(()=>client.dispose());
  assert.match(await client.call(['search','quartz-token','--json'],undefined,{cwd:f.cwd}),/desktop-demo/);
  assert.match(await mcp(client,f.cwd,'recall','shared-jade'),/shared-jade/);
  assert.doesNotMatch(await mcp(client,f.cwd,'remember','Synthetic shared-ruby note from plugin','explicit-project'),/"isError":true/);
  await f.cli(['index']);
  assert.match(await f.cli(['search','shared-ruby','--json']),/shared-ruby/);
  const notes=readFileSync(f.env.DEJA_NOTES_FILE,'utf8');
  assert.match(notes,/shared-jade/);assert.match(notes,/shared-ruby/);
  const {existsSync}=await import('node:fs');
  assert.equal(existsSync(join(f.root,'.cache','deja','index.db')),false);
  assert.equal(existsSync(join(f.root,'config','deja','notes.jsonl')),false);
});

test('shared policy changes apply to both extension hooks and configured MCP', {skip:!binary},async t=>{
  const f=fixture(t);await f.cli(['index']);
  const output=join(f.root,'policy-plugin');
  const policyFile=join(f.env.XDG_CONFIG_HOME,'deja','policy.json');
  mkdirSync(join(f.env.XDG_CONFIG_HOME,'deja'),{recursive:true});
  writeFileSync(policyFile,'{}');
  configure({binary:resolve(binary),output,environment:f.env,home:f.root});
  const client=new DejaClient(loadConfig(output));t.after(()=>client.dispose());
  assert.equal(client.config.env.DEJA_POLICY_FILE,policyFile);
  assert.match(await mcp(client,f.cwd,'recall','quartz-token'),/quartz-token/);
  const handlers=new Map();attach({on:(name,handler)=>handlers.set(name,handler)},client);
  const ctx=id=>({cwd:f.cwd,sessionManager:{getSessionId:()=>id},ui:{setStatus(){}}});
  assert.match((await handlers.get('before_agent_start')({prompt:'quartz-token',systemPrompt:'BASE'},ctx('policy-allowed'))).systemPrompt,/quartz-token/);
  writeFileSync(policyFile,JSON.stringify({activations:{auto:{local:false,imported:false},mcp:{local:false,imported:false}},auto_index:false}));
  const raw=await mcp(client,f.cwd,'recall','quartz-token');
  const reply=raw.split('\n').map(line=>JSON.parse(line)).find(message=>message.id===2);
  assert.doesNotMatch(JSON.stringify(reply.result),/desktop-demo|Decided to invalidate quartz-token/);
  const result=await handlers.get('before_agent_start')({prompt:'quartz-token',systemPrompt:'BASE'},ctx('policy-allowed'));
  assert.doesNotMatch(result?.systemPrompt??'',/desktop-demo|Decided to invalidate quartz-token/);
  assert.match(await f.cli(['search','quartz-token','--json']),/desktop-demo/);
});

test('new MCP bridge recalls and remembers through the external engine and shared data', {skip:!binary},async t=>{
  const f=fixture(t);await f.cli(['index']);
  const output=join(f.root,'bridge-plugin');configure({binary:resolve(binary),output,environment:f.env,home:f.root});
  const config=loadConfig(output), options={resolve:()=>{config.refresh();return config;}};
  const recalled=await forward({mode:'recall',q:'quartz-token',cwd:f.cwd},options);
  assert.match(JSON.stringify(recalled),/quartz-token/);
  const remembered=await forward({mode:'remember',q:'Synthetic bridge-opal decision',project:'explicit-project',cwd:f.cwd},options);
  assert.notEqual(remembered.isError,true);assert.match(readFileSync(f.env.DEJA_NOTES_FILE,'utf8'),/bridge-opal/);
  await f.cli(['index']);assert.match(await f.cli(['search','bridge-opal','--json']),/bridge-opal/);
});

test('staged manifest entry runs a real host-style MCP handshake and external recall', {skip:!binary},async t=>{
  const f=fixture(t);await f.cli(['index']);
  const output=join(f.root,'host-plugin');const manifest=configure({binary:resolve(binary),output,environment:f.env,home:f.root});
  const {spawn}=await import('node:child_process');const server=manifest.contributes.mcpServers[0];
  const child=spawn(process.execPath,server.args,{cwd:output,env:{...process.env,DEJA_PI_CONFIG_FILE:roots.get(output).file,USERPROFILE:f.root,HOME:f.root},stdio:['pipe','pipe','pipe'],windowsHide:true});
  let buffer='',stderr='';const pending=new Map();child.stderr.on('data',data=>{stderr+=data;});
  child.stdout.setEncoding('utf8');child.stdout.on('data',data=>{buffer+=data;let end;while((end=buffer.indexOf('\n'))>=0){const reply=JSON.parse(buffer.slice(0,end));buffer=buffer.slice(end+1);pending.get(reply.id)?.(reply);}});
  const closed=new Promise(resolve=>child.once('close',resolve));
  f.cleanup.push(async()=>{child.stdin.end();const timeout=setTimeout(()=>child.kill(),3000);await closed;clearTimeout(timeout);});
  const call=message=>Promise.race([new Promise(resolve=>{pending.set(message.id,resolve);child.stdin.write(JSON.stringify({jsonrpc:'2.0',...message})+'\n');}),new Promise((_resolve,reject)=>{const timer=setTimeout(()=>reject(new Error('bridge timeout '+stderr)),10000);timer.unref();})]);
  assert.ok((await call({id:1,method:'initialize',params:{protocolVersion:'2025-06-18'}})).result);
  child.stdin.write(JSON.stringify({jsonrpc:'2.0',method:'notifications/initialized'})+'\n');
  assert.equal((await call({id:2,method:'tools/list'})).result.tools[0].name,'deja');
  const result=await call({id:3,method:'tools/call',params:{name:'deja',arguments:{mode:'recall',q:'quartz-token',cwd:f.cwd}}});
  assert.notEqual(result.result.isError,true,JSON.stringify(result));assert.match(JSON.stringify(result),/quartz-token/);
});

test('MCP bridge remembers under the actual project cwd when project is omitted', {skip:!binary},async t=>{
  const f=fixture(t);await f.cli(['index']);const output=join(f.root,'cwd-plugin');
  configure({binary:resolve(binary),output,environment:f.env,home:f.root});const config=loadConfig(output);
  const result=await forward({mode:'remember',q:'Synthetic project-cwd-zircon decision',cwd:f.cwd},{resolve:()=>config});
  assert.notEqual(result.isError,true);const notes=readFileSync(f.env.DEJA_NOTES_FILE,'utf8').trim().split('\n').map(line=>JSON.parse(line));
  const note=notes.find(note=>note.text.includes('project-cwd-zircon'));assert.ok(note);
  assert.match(note.project,/cache/);assert.doesNotMatch(note.project,/cwd-plugin|pi-desktop-plugin/);
});

test('real tool hooks accept Desktop payloads without altering original results', {skip:!binary},async t=>{
  const f=fixture(t);await f.cli(['index']);const client=new DejaClient({executable:binary,env:f.env});f.cleanup.push(()=>client.dispose());
  const handlers=new Map();attach({on:(name,handler)=>handlers.set(name,handler)},client);
  const ctx={cwd:f.cwd,sessionManager:{getSessionId:()=> 'tool-reader'},ui:{setStatus(){}}};
  const read={toolName:'Read',toolCallId:'read-actual',input:{path:join(f.cwd,'src','cache.ts')},content:[{type:'text',text:'original read'}]};
  const failure={toolName:'Bash',toolCallId:'fail-actual',input:{command:'go test ./...'},content:[{type:'text',text:'synthetic unknown error'}],isError:true,details:{exitCode:1}};
  const before=structuredClone(failure);await handlers.get('tool_result')(read,ctx);await handlers.get('tool_result')(failure,ctx);assert.deepEqual(failure,before);
  await handlers.get('session_shutdown')({},ctx);assert.equal(client.pending.size,0);
});
