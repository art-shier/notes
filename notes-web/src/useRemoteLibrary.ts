import {useEffect,useMemo,useRef,useState} from 'react';
import {api,toNote,type Account} from './api';
import type {Library,Note} from './data';
import {SaveQueue,type Patch,type SaveState} from './saveQueue';
import {DraftStore,type Draft} from './draftStore';
export type {Draft} from './draftStore';
export function useRemoteLibrary(account:Account){
  const [library,setLibrary]=useState<Library>({folders:[],notes:[]});const [loading,setLoading]=useState(true);const [loadError,setLoadError]=useState('');
  const [states,setStates]=useState<Record<string,{state:SaveState;message?:string}>>({});const [draftWarning,setDraftWarning]=useState(false);
  const alive=useRef(true);
  const storage=useMemo(()=>new DraftStore(account.id,localStorage,crypto.randomUUID()),[account.id]);
  const queue=useMemo(()=>new SaveQueue(
    api.patch,
    (id,state,error)=>{if(alive.current)setStates(s=>({...s,[id]:{state,message:error?(error as Error).message:undefined}}))},
    (id,result)=>{if(alive.current){const saved=toNote(result);setLibrary(s=>({...s,notes:s.notes.map(n=>n.id===id?{...n,version:saved.version,updatedAt:saved.updatedAt,source:saved.source}:n)}))}},
    (id,version,patch)=>{try{storage.write(id,version,patch)}catch{if(alive.current)setDraftWarning(true)}}
  ),[account.id]);
  async function reload(){setLoading(true);setLoadError('');try{const data=await api.library();if(alive.current){data.notes.forEach(n=>queue.adopt(n.id,n.version||1));setLibrary(data)}}catch(e){if(alive.current)setLoadError((e as Error).message)}finally{if(alive.current)setLoading(false)}}
  useEffect(()=>{alive.current=true;void reload();return()=>{alive.current=false;queue.dispose()}},[queue]);
  useEffect(()=>{const before=(e:BeforeUnloadEvent)=>{if(queue.hasPending()){e.preventDefault();e.returnValue=''}};window.addEventListener('beforeunload',before);return()=>window.removeEventListener('beforeunload',before)},[queue]);
  const putNote=(note:Note)=>{queue.adopt(note.id,note.version||1);setLibrary(s=>({...s,notes:s.notes.some(n=>n.id===note.id)?s.notes.map(n=>n.id===note.id?note:n):[note,...s.notes]}))};
  const update=(id:string,patch:Partial<Note>)=>{
    const note=library.notes.find(n=>n.id===id);if(!note)return;
    const remote:Patch={};if('title'in patch)remote.title=patch.title;if('content'in patch)remote.content_json=patch.content;if('folderId'in patch)remote.folder_id=patch.folderId;if('favorite'in patch)remote.favorite=patch.favorite;if('tags'in patch)remote.tag_ids=patch.tags?.map(t=>t.id)||[];
    setLibrary(s=>({...s,notes:s.notes.map(n=>n.id===id?{...n,...patch}:n)}));queue.stage(id,note.version||1,remote);
  };
  const readDraft=(id:string):Draft|null=>{try{return storage.newest(id)}catch{return null}};
  const discard=(id:string,draft?:Draft)=>{queue.discard(id);storage.discard(id,draft);setStates(s=>({...s,[id]:{state:'saved'}}))};
  const restore=(id:string,draft:Draft)=>{
    storage.recover(id,draft);queue.discard(id);
    const r=draft.patch;setLibrary(s=>({...s,notes:s.notes.map(n=>n.id===id?{...n,...('title'in r?{title:r.title as string}:{}),...('content_json'in r?{content:r.content_json as Note['content']}:{}),...('folder_id'in r?{folderId:r.folder_id as string}:{}),...('favorite'in r?{favorite:r.favorite as boolean}:{}),...('tag_ids'in r?{tags:(r.tag_ids as string[]).map(tid=>s.tags?.find(t=>t.id===tid)||n.tags?.find(t=>t.id===tid)||{id:tid,name:'未识别标签'})}:{})}:n)}));queue.stage(id,draft.version,draft.patch);
  };
  return {library,setLibrary,loading,loadError,reload,queue,states,putNote,update,readDraft,discard,restore,draftWarning};
}
