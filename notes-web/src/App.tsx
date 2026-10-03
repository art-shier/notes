import {lazy,Suspense,useEffect,useRef,useState} from 'react';
import {X,FolderPlus,AlertCircle,Check} from 'lucide-react';
import {FolderBrowser,type View} from './FolderBrowser';
import {Settings} from './Settings';
import {AuthPage} from './AuthPage';
import {api,ApiError,type Account,type ExportOptions} from './api';
import {useRemoteLibrary,type Draft} from './useRemoteLibrary';
import type {Note} from './data';
const NoteEditor=lazy(()=>import('./NoteEditor').then(m=>({default:m.NoteEditor})));
const HistoryPanel=lazy(()=>import('./HistoryPanel').then(m=>({default:m.HistoryPanel})));
export function App(){
  const [account,setAccount]=useState<Account|null>(null);const [ready,setReady]=useState(false);const [error,setError]=useState('');
  const [theme,setTheme]=useState<'light'|'dark'>(()=>{try{return localStorage.getItem('shiji.theme')==='dark'?'dark':'light'}catch{return 'light'}});
  useEffect(()=>{document.documentElement.dataset.theme=theme;try{localStorage.setItem('shiji.theme',theme)}catch{}},[theme]);
  const identify=async()=>{setReady(false);setError('');try{setAccount(await api.me())}catch(e){if(!(e instanceof ApiError&&e.status===401))setError((e as Error).message)}finally{setReady(true)}};
  useEffect(()=>{void identify()},[]);
  if(!ready)return <main className="app-shell"><div className="app-content empty-state" role="status">正在打开笔记空间…</div></main>;
  if(error)return <main className="app-shell"><div className="app-content empty-state"><h2>暂时无法连接服务</h2><p>{error}</p><button className="secondary-button" onClick={()=>void identify()}>重新连接</button></div></main>;
  if(!account)return <main className="app-shell"><div className="app-content"><AuthPage onLogin={setAccount}/></div></main>;
  return <Workspace key={account.id} account={account} theme={theme} onTheme={()=>setTheme(t=>t==='light'?'dark':'light')} onLogout={()=>setAccount(null)}/>;
}
function Workspace({account,theme,onTheme,onLogout}:{account:Account;theme:'light'|'dark';onTheme:()=>void;onLogout:()=>void}){
  const remote=useRemoteLibrary(account);const {library,setLibrary,queue,states}=remote;
  const [folderId,setFolderId]=useState<string|null>(null);const [view,setView]=useState<View>('folders');const [query,setQuery]=useState('');
  const [page,setPage]=useState<'library'|'note'|'settings'|'history'>('library');const [selected,setSelected]=useState<string|null>(null);const [editorKey,setEditorKey]=useState(0);
  const [message,setMessage]=useState('');const [folderName,setFolderName]=useState('');const [folderError,setFolderError]=useState('');const [busy,setBusy]=useState(false);const busyRef=useRef(false);
  const [recovery,setRecovery]=useState<{id:string;draft:Draft}|null>(null);const modal=useRef<HTMLDialogElement>(null);
  const [search,setSearch]=useState<{key:string;notes:Note[]}|null>(null);
  const searchKey=JSON.stringify([query,folderId,view]);
  useEffect(()=>{if(!message)return;const t=setTimeout(()=>setMessage(''),4500);return()=>clearTimeout(t)},[message]);
  useEffect(()=>{window.scrollTo({top:0})},[page,folderId]);
  useEffect(()=>{if(!query){setSearch(null);return}let live=true;const t=setTimeout(()=>{void api.search(query,folderId,view).then(notes=>{if(live)setSearch({key:searchKey,notes})}).catch(e=>{if(live)setMessage(e.message)})},250);return()=>{live=false;clearTimeout(t)}},[searchKey]);
  const note=library.notes.find(n=>n.id===selected);const save=selected?states[selected]:undefined;
  async function run(action:()=>Promise<void>|void,flush=true){if(busyRef.current)return;busyRef.current=true;setBusy(true);try{if(flush&&!await queue.flushAll()){setMessage('有笔记尚未保存，请先处理保存提示。');return}await action()}catch(e){setMessage((e as Error).message)}finally{busyRef.current=false;setBusy(false)}}
  const openNote=(id:string)=>void run(async()=>{const n=await api.note(id);remote.putNote(n);setSelected(id);setEditorKey(k=>k+1);setRecovery(remote.readDraft(id)?{id,draft:remote.readDraft(id)!}:null);setPage('note')});
  const openFolder=(id:string|null)=>void run(()=>{setFolderId(id);setView('folders');setQuery('')});
  const addNote=()=>void run(async()=>{const inbox=library.folders.find(f=>f.isInbox);if(!inbox)throw new Error('收件箱未加载，请重试。');const n=await api.createNote(folderId||inbox.id);remote.putNote(n);setSelected(n.id);setEditorKey(k=>k+1);setRecovery(null);setPage('note')});
  function download(value:unknown,name:string){const url=URL.createObjectURL(new Blob([JSON.stringify(value,null,2)],{type:'application/json'}));const a=document.createElement('a');a.href=url;a.download=name;a.click();setTimeout(()=>URL.revokeObjectURL(url),1000)}
  const exportNotes=async(options:ExportOptions)=>{if(!await queue.flushAll())throw new Error('有笔记尚未保存，请先处理保存提示。');const bundle=await api.prepareExport(options);api.downloadExport(bundle);setMessage('图文包已准备，浏览器开始下载。');return bundle};
  const refreshCurrent=async()=>{if(!note)return;if(!await queue.flushAll())throw new Error('请先处理未保存的内容。');remote.putNote(await api.note(note.id))};
  const restoreHistory=async(version:number)=>{if(!note)return;if(!await queue.flushAll())throw new Error('请先处理未保存的内容。');const restored=await api.restoreRevision(note.id,version,queue.versionFor(note.id,note.version||1));remote.putNote(restored);setEditorKey(k=>k+1);setRecovery(null);setPage('note');setMessage('标题和正文已恢复，恢复前的内容仍在历史中。')};
  return <main className="app-shell" aria-busy={busy}><div className="app-content">
    {remote.draftWarning&&<div className="save-error" role="alert"><AlertCircle size={17}/><span>浏览器无法保存草稿，请保持页面打开直到服务器保存成功。</span></div>}
    {remote.loading?<div className="empty-state" role="status">正在加载你的笔记…</div>:remote.loadError?<div className="empty-state"><h2>笔记加载失败</h2><p>{remote.loadError}</p><button className="secondary-button" onClick={()=>void remote.reload()}>重新加载</button></div>:
    page==='library'?<FolderBrowser library={library} folderId={folderId} view={view} query={query} results={search?.key===searchKey?search.notes:undefined} searching={!!query&&search?.key!==searchKey} onQuery={setQuery} onView={v=>{setView(v);setQuery('')}} onFolder={openFolder} onNote={openNote} onNew={addNote} onAddFolder={()=>{setFolderName('');setFolderError('');modal.current?.showModal()}} onSettings={()=>void run(()=>setPage('settings'))} theme={theme} onTheme={onTheme} displayName={account.display_name}/>:
    page==='settings'?<Settings account={account} onBack={()=>void run(()=>setPage('library'))} onTrash={()=>void run(()=>{setFolderId(null);setView('trash');setQuery('');setPage('library')})} onExport={exportNotes} onLogout={()=>void run(async()=>{await api.logout();onLogout()})} notify={setMessage}/>:
    page==='history'&&note?<Suspense fallback={<div className="empty-state">正在打开历史记录…</div>}><HistoryPanel key={note.id} note={note} onBack={()=>{setEditorKey(k=>k+1);setPage('note')}} onRestore={restoreHistory} onRefresh={refreshCurrent}/></Suspense>:
    note&&<>
      {recovery&&<div className="draft-notice" role="alert"><strong>发现这篇笔记的未保存草稿</strong><p>草稿只属于当前账户。恢复后仍会检查服务器版本。</p><div className="action-group"><button className="secondary-button" onClick={()=>{remote.restore(recovery.id,recovery.draft);setEditorKey(k=>k+1);setRecovery(null)}}>恢复草稿</button><button className="text-button" onClick={()=>{if(confirm('放弃这份未保存草稿，使用服务器版本？')){remote.discard(recovery.id,recovery.draft);setEditorKey(k=>k+1);setRecovery(null)}}}>使用服务器版本</button></div></div>}
      {save&&(save.state==='error'||save.state==='conflict')&&<div className="draft-notice" role="alert"><strong>{save.state==='conflict'?'保存冲突，已保留本地内容':'保存失败，已保留本地草稿'}</strong><p>{save.message}</p><div className="action-group">{save.state==='error'&&<button className="secondary-button" onClick={()=>void queue.flush(note.id,true)}>重试保存</button>}<button className="secondary-button" onClick={()=>download(note,'shiji-draft.json')}>下载本地草稿</button><button className="text-button" onClick={()=>{if(confirm('放弃页面内修改并重新加载服务器版本？请先下载要保留的草稿。'))void run(async()=>{const n=await api.note(note.id);remote.discard(note.id);remote.putNote(n);setEditorKey(k=>k+1)},false)}}>重新加载</button></div></div>}
      <Suspense fallback={<div className="empty-state">正在打开编辑器…</div>}><NoteEditor key={note.id+':'+editorKey} note={note} folders={library.folders} tags={library.tags||[]} onCreateTag={tag=>setLibrary(s=>({...s,tags:[...(s.tags||[]),tag]}))} onBack={()=>void run(async()=>{await remote.reload();setPage('library')})} onHistory={()=>void run(async()=>{await refreshCurrent();setPage('history')})} onChange={patch=>remote.update(note.id,patch)} onTrash={()=>void run(async()=>{remote.putNote(await api.trash({...note,version:queue.versionFor(note.id,note.version||1)}));setPage('library');setMessage(note.trashed?'已恢复笔记。':'已移入回收站。')})} saveState={save?.state||'saved'} disabled={!!recovery} notify={setMessage}/></Suspense>
    </>}
    {busy&&<div className="busy-status" role="status">正在处理…</div>}
  </div>
  <dialog ref={modal} className="folder-dialog"><form onSubmit={e=>{e.preventDefault();void run(async()=>{try{const f=await api.folder(folderName.trim(),folderId);setLibrary(s=>({...s,folders:[...s.folders,f]}));modal.current?.close();setMessage('文件夹已创建。')}catch(e){setFolderError((e as Error).message)}},false)}}><div className="dialog-heading"><FolderPlus size={23}/><h2>新建文件夹</h2><button type="button" className="icon-button" aria-label="关闭" onClick={()=>modal.current?.close()}><X size={20}/></button></div><p>放在{library.folders.find(f=>f.id===folderId)?.name||'笔记库'}中。</p><label className="form-field">文件夹名称<input autoFocus required maxLength={80} value={folderName} onChange={e=>{setFolderName(e.target.value);setFolderError('')}} aria-invalid={!!folderError}/></label><p className="field-error" role="alert">{folderError}</p><button type="submit" className="primary-button full-width" disabled={busy}>创建文件夹</button></form></dialog>
  {message&&<div className="toast" role="status"><Check size={16}/>{message}</div>}
  </main>;
}

