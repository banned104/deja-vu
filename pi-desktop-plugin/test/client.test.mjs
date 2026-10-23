import test from 'node:test';
import assert from 'node:assert/strict';
import { runProcess, DejaClient, loadConfig } from '../src/deja-client.mjs';

const node = (script, options = {}) => runProcess(process.execPath, ['-e', script], options);

test('process client sends JSON via stdin without shell interpolation',async()=>{
  const value = 'quote " and ; & 中文';
  const out=await node('let b="";process.stdin.on("data",x=>b+=x);process.stdin.on("end",()=>process.stdout.write(b))',{input:JSON.stringify({value})});
  assert.deepEqual(JSON.parse(out),{value});
});

test('process client reports nonzero status and missing binary',async()=>{
  await assert.rejects(node('process.stderr.write("fixture error");process.exit(7)'),/exited 7/);
  await assert.rejects(runProcess('nonexistent-deja-fixture-44921',[]),/ENOENT/);
});

test('process timeout waits for child cleanup',async()=>{
  await assert.rejects(node('setInterval(()=>{},1000)',{timeout:120}),/timed out/);
});

test('abort and output budgets stop the owned process',async()=>{
  const c=new AbortController();
  const pending=node('setInterval(()=>{},1000)',{signal:c.signal});
  c.abort(); await assert.rejects(pending,/cancelled/);
  await assert.rejects(node('process.stdout.write("x".repeat(1000000));setInterval(()=>{},1000)',{maxBytes:100}),/budget/);
});

test('dispose cancels background work; turn cancellation does not own warmup',async()=>{
  const seen=[];
  const run=(_exe,_args,options)=>new Promise((_resolve,reject)=>{
    seen.push(options);
    options.signal.addEventListener('abort',()=>reject(new Error('cancelled')),{once:true});
  });
  const client=new DejaClient({executable:'fixture',env:{}},run);
  const turn=new AbortController();
  client.prepare({cwd:process.cwd(),signal:turn.signal},()=>{});
  turn.abort(); assert.equal(seen[0].signal.aborted,false);
  await client.dispose(); assert.equal(seen[0].signal.aborted,true);
  assert.equal(client.pending.size,0);
});

test('missing external engine fails with a settings command hint',async()=>{
  const client=new DejaClient({executable:'',env:{}});
  await assert.rejects(client.call(['hook-context']),/\/deja/);
  await client.dispose();
});

test('background indexing is quiet and uses an independent output budget',async()=>{
  let seen;
  const client=new DejaClient({executable:'fixture',env:{}},async(_exe,args,options)=>{seen={args,options};return ''});
  client.prepare({cwd:process.cwd()},()=>{});
  await client.warmup;
  assert.deepEqual(seen.args,['index','--quiet']);
  assert.equal(seen.options.maxBytes,1024*1024);
  await client.dispose();
});

test('cancellation terminates the child process tree, not just its parent',async()=>{
  const {mkdtempSync,readFileSync,rmSync,watch}=await import('node:fs');
  const {join}=await import('node:path');const {tmpdir}=await import('node:os');
  const dir=mkdtempSync(join(process.env.PI_SCRATCH_DIR??tmpdir(),'deja-tree-test-'));
  const path=join(dir,'pid');const controller=new AbortController();
  let watcher,descendant;
  const ready=new Promise(resolve=>{watcher=watch(dir,()=>{try{descendant=Number(readFileSync(path,'utf8'));if(descendant)resolve();}catch{}})});
  const script=`const {spawn}=require('node:child_process');const fs=require('node:fs');const c=spawn(process.execPath,['-e','setInterval(()=>{},1000)'],{stdio:'ignore'});fs.writeFileSync(${JSON.stringify(path)},String(c.pid));setInterval(()=>{},1000)`;
  const pending=node(script,{signal:controller.signal,timeout:5000});
  pending.catch(()=>{});
  try {
    await Promise.race([ready,pending]);
    controller.abort();await assert.rejects(pending,/cancelled/);
    assert.throws(()=>process.kill(descendant,0));
  } finally {
    controller.abort();watcher?.close();
    if(descendant){try{process.kill(descendant,'SIGKILL')}catch{}}
    await Promise.allSettled([pending]);rmSync(dir,{recursive:true,force:true});
  }
});
