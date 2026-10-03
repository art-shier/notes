import {test} from 'node:test';
import assert from 'node:assert/strict';
import {SaveQueue} from './saveQueue.ts';

test('serial save keeps the newest edits and advances expected_version',async()=>{
  const calls:Array<{version:number;patch:any}>=[];let release:()=>void=()=>{};
  const gate=new Promise<void>(resolve=>release=resolve);
  const q=new SaveQueue(async(id,version,patch)=>{calls.push({version,patch});if(calls.length===1)await gate;return {version:version+1}},()=>{},()=>{},()=>{});
  q.stage('a',1,{title:'first'});const running=q.flush('a');
  q.stage('a',1,{title:'latest'});release();assert.equal(await running,true);
  assert.deepEqual(calls,[{version:1,patch:{title:'first'}},{version:2,patch:{title:'latest'}}]);q.dispose();
});
test('failed save preserves latest draft and conflicts do not silently retry',async()=>{
  const drafts:any[]=[];let count=0;
  const q=new SaveQueue(async()=>{count++;throw Object.assign(new Error('conflict'),{status:409})},()=>{},()=>{},(id,version,patch)=>drafts.push({id,version,patch}));
  q.stage('a',3,{title:'local'});assert.equal(await q.flush('a'),false);
  q.stage('a',3,{title:'newer local'});assert.equal(await q.flush('a'),false);assert.equal(count,1);
  assert.deepEqual(drafts.at(-1),{id:'a',version:3,patch:{title:'newer local'}});q.dispose();
});
test('network retry keeps pending edits and clears acknowledged drafts',async()=>{
  let fail=true;let cleared=false;
  const q=new SaveQueue(async(id,version)=>{if(fail)throw new Error('offline');return {version:version+1}},()=>{},()=>{},(id,version,patch)=>{cleared=patch===null});
  q.stage('a',1,{title:'persist'});assert.equal(await q.flushAll(),false);fail=false;
  assert.equal(await q.flush('a',true),true);assert.equal(cleared,true);q.dispose();
});
test('edits during a request retain both unacknowledged fields in the recovery draft',async()=>{
  const drafts:any[]=[];let release:()=>void=()=>{};const gate=new Promise<void>(r=>release=r);
  const q=new SaveQueue(async(id,version)=>{await gate;return {version:version+1}},()=>{},()=>{},(id,version,patch)=>drafts.push(patch));
  q.stage('a',1,{title:'title in flight'});const running=q.flush('a');q.stage('a',1,{content_json:{type:'doc'}});
  assert.deepEqual(drafts.at(-1),{title:'title in flight',content_json:{type:'doc'}});release();await running;q.dispose();
});
test('authoritative reload rebases idle records but never pending edits',async()=>{
  const versions:number[]=[];const q=new SaveQueue(async(id,version)=>{versions.push(version);return {version:version+1}},()=>{},()=>{},()=>{});
  q.stage('a',1,{title:'first'});await q.flush('a');q.adopt('a',4);q.stage('a',4,{title:'after reload'});await q.flush('a');
  q.stage('a',5,{title:'pending'});q.adopt('a',9);await q.flush('a');assert.deepEqual(versions,[1,4,5]);q.dispose();
});
