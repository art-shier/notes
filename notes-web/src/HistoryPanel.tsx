import {useEffect,useState} from 'react';
import {useEditor,EditorContent} from '@tiptap/react';
import {ArrowLeft,ChevronRight,History,RotateCcw,RefreshCw,Bot,Globe} from 'lucide-react';
import {api,ApiError,type Revision,type RevisionDetail} from './api';
import {editorExtensions} from './editorExtensions';
import type {Note} from './data';

const actions:Record<string,string>={create:'创建笔记',update:'保存修改',blocks:'按块修改',trash:'移入回收站',restore:'恢复笔记',history_restore:'恢复历史正文',tag_delete:'解除已删除标签',migration:'升级时保留的版本'};
const time=(value:string)=>new Intl.DateTimeFormat('zh-CN',{month:'numeric',day:'numeric',hour:'2-digit',minute:'2-digit',timeZone:'Asia/Shanghai'}).format(new Date(value));

export function HistoryPanel({note,onBack,onRestore,onRefresh}:{note:Note;onBack:()=>void;onRestore:(version:number)=>Promise<void>;onRefresh:()=>Promise<void>}){
  const [rows,setRows]=useState<Revision[]>([]);const [cursor,setCursor]=useState<number|null>(null);const [limit,setLimit]=useState(200);const [current,setCurrent]=useState(note.version||1);
  const [loading,setLoading]=useState(true);const [busy,setBusy]=useState(false);const [error,setError]=useState('');const [conflict,setConflict]=useState(false);const [selected,setSelected]=useState<RevisionDetail|null>(null);
  async function load(before?:number){setLoading(true);setError('');try{const page=await api.history(note.id,before);setRows(prev=>before?[...prev,...page.items]:page.items);setCursor(page.next_before_version);setCurrent(page.current_version);setLimit(page.retention_limit)}catch(e){setError((e as Error).message)}finally{setLoading(false)}}
  useEffect(()=>{void load()},[note.id]);
  const preview=async(version:number)=>{if(busy)return;setBusy(true);setError('');setConflict(false);try{setSelected(await api.revision(note.id,version))}catch(e){setError((e as Error).message)}finally{setBusy(false)}};
  const restore=async()=>{if(!selected||busy||!confirm('恢复这个版本的标题和正文？恢复前的内容仍保留在历史中，当前文件夹、标签和收藏不变。'))return;setBusy(true);setError('');setConflict(false);try{await onRestore(selected.version)}catch(e){setError((e as Error).message);setConflict(e instanceof ApiError&&e.status===409)}finally{setBusy(false)}};
  return <section className="history-page" aria-busy={busy||loading}>
    <header className="history-header"><button className="back-button" disabled={busy} onClick={()=>{if(selected){setSelected(null);setError('');setConflict(false)}else onBack()}}><ArrowLeft size={18}/>{selected?'版本列表':'返回笔记'}</button><h1>{selected?'版本 '+selected.version:'历史版本'}</h1><p className="header-subtitle">{selected?time(selected.saved_at)+' · '+(selected.actor==='agent'?'Agent 保存':selected.actor==='system'?'系统保留':'网页保存'):`保留最近 ${limit} 个已保存版本。旧笔记从升级时开始记录。`}</p></header>
    {error&&<div className="draft-notice" role="alert"><strong>{conflict?'当前笔记已发生变化':'操作未完成'}</strong><p>{error}</p>{conflict?<button className="secondary-button" disabled={busy} onClick={async()=>{setBusy(true);try{await onRefresh();await load();setConflict(false);setError('')}catch(e){setError((e as Error).message)}finally{setBusy(false)}}}><RefreshCw size={15}/>刷新当前版本</button>:<button className="text-button" disabled={busy} onClick={()=>void load()}>重新加载版本列表</button>}</div>}
    {selected?<>
      <div className="revision-preview-meta"><span>{actions[selected.action]||'保存版本'}</span>{selected.restored_from&&<span>从版本 {selected.restored_from} 恢复</span>}{selected.version===current&&<span className="status-badge">当前版本</span>}</div>
      <h2 className="revision-title">{selected.snapshot.title||'未命名笔记'}</h2>
      <ReadOnlyDocument key={selected.version} note={selected.snapshot}/>
      <div className="inline-notice"><History size={16}/>预览只读。恢复只替换标题和正文，当前文件夹、标签和收藏继续保留。</div>
      {note.trashed?<p className="settings-copy">先从回收站恢复笔记，再恢复历史正文。</p>:<button className="primary-button full-width" disabled={busy||selected.version===current} onClick={()=>void restore()}><RotateCcw size={17}/>{busy?'正在恢复…':selected.version===current?'这是当前版本':'恢复此版本的正文'}</button>}
    </>:<>
      {!rows.length&&loading?<div className="empty-state" role="status">正在读取历史版本…</div>:!rows.length&&!error?<div className="empty-state"><History size={30}/><h2>还没有历史记录</h2><p>成功保存后，会在这里留下版本。</p></div>:<div className="revision-list">{rows.map(row=><button className="revision-row" key={row.version} disabled={busy||loading} onClick={()=>void preview(row.version)}><span className="revision-symbol">{row.actor==='agent'?<Bot size={18}/>:row.actor==='system'?<History size={18}/>:<Globe size={18}/>}</span><span className="revision-row-copy"><strong>版本 {row.version}{row.version===current&&<small>当前</small>}</strong><span>{time(row.saved_at)} · {actions[row.action]||'保存修改'}</span><p>{row.title||'未命名笔记'}{row.excerpt?' · '+row.excerpt:''}</p></span><ChevronRight size={16}/></button>)}</div>}
      {cursor!==null&&<button className="secondary-button full-width" disabled={busy||loading} onClick={()=>void load(cursor)}>{loading?'正在加载…':'更早的版本'}</button>}
    </>}
  </section>;
}

function ReadOnlyDocument({note}:{note:Note}){
  const editor=useEditor({extensions:editorExtensions(),content:note.content,editable:false});
  return <EditorContent editor={editor} className="document-body revision-document" aria-label="历史正文（只读）"/>;
}
