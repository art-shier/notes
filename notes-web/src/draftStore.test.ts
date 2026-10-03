import {test} from 'node:test';import assert from 'node:assert/strict';
import {DraftStore} from './draftStore.ts';
const memory=()=>{const map=new Map<string,string>();return {get length(){return map.size},key:(i:number)=>[...map.keys()][i]||null,getItem:(k:string)=>map.get(k)||null,setItem:(k:string,v:string)=>{map.set(k,v)},removeItem:(k:string)=>{map.delete(k)}}};
test('one editor acknowledgment cannot clear another editor recovery draft',()=>{
 const storage=memory(),a=new DraftStore('user',storage,'a'),b=new DraftStore('user',storage,'b');
 a.write('note',1,{title:'A'});b.write('note',1,{title:'B'});a.write('note',2,null);assert.equal(b.newest('note')?.patch.title,'B');assert.equal(new DraftStore('other',storage,'c').newest('note'),null);
});
test('recovery clears source only after successful acknowledgement and only if unchanged',()=>{
 const storage=memory(),a=new DraftStore('user',storage,'a'),b=new DraftStore('user',storage,'b');a.write('note',1,{title:'old'});const draft=b.newest('note')!;b.recover('note',draft);b.write('note',1,draft.patch);assert.equal(storage.length,2);
 a.write('note',1,{title:'newer in other tab'});b.write('note',2,null);assert.equal(a.newest('note')?.patch.title,'newer in other tab');
 const next=b.newest('note')!;b.recover('note',next);b.write('note',1,next.patch);b.write('note',2,null);assert.equal(storage.length,0);
});
