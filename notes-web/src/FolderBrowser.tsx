import { ChevronRight, Folder as FolderIcon, Inbox, Search, FolderPlus, Plus, Star, Bot, ArrowLeft, NotebookPen, Trash2, Sun, Moon } from 'lucide-react';
import { useMemo } from 'react';
import { descendants, plainText, stamp, type Library, type Note } from './data';
export type View = 'folders' | 'favorites' | 'agent' | 'trash';
interface Props { library: Library; folderId: string | null; view: View; query: string; results?:Note[]; searching?:boolean; onQuery: (s:string)=>void; onView:(s:View)=>void; onFolder:(id:string|null)=>void; onNote:(id:string)=>void; onNew:()=>void; onAddFolder:()=>void; onSettings:()=>void; theme: 'light' | 'dark'; onTheme:()=>void; displayName?:string; }
export function FolderBrowser(p: Props) {
  const { library, folderId, view, query } = p;
  const folder = library.folders.find(f => f.id === folderId);
  const scope = folderId ? descendants(library.folders, folderId) : null;
  const notes = useMemo(() => query && p.results ? p.results : library.notes.filter(n =>
    !!n.trashed === (view === 'trash') && (!scope || scope.includes(n.folderId)) &&
    (view !== 'favorites' || n.favorite) && (view !== 'agent' || n.source === 'agent') &&
    (!query || (n.title + ' ' + (n.excerpt||plainText(n.html))).toLowerCase().includes(query.toLowerCase()))
  ).sort((a,b) => b.updatedAt.localeCompare(a.updatedAt)), [library,folderId,view,query,p.results]);
  const folders = library.folders.filter(f => f.parentId === folderId);
  const grouped = view === 'folders' && !query;
  const direct = notes.filter(n => n.folderId === folderId);
  const viewName = view === 'favorites' ? '收藏' : view === 'agent' ? 'Agent 创建' : view === 'trash' ? '回收站' : '文件夹';
  const noteList = (items: Note[]) => <div className="note-list">{items.map(n => {
    const image = n.thumbnail || new DOMParser().parseFromString(n.html, 'text/html').querySelector('img')?.getAttribute('src');
    return <button key={n.id} className="note-row" onClick={() => p.onNote(n.id)}>
      <div className="note-row-copy"><div className="note-row-top"><h2>{n.title || '未命名笔记'}</h2>{n.favorite && <Star size={14} className="favorite-mark"/>}</div>
      <p>{n.excerpt || plainText(n.html) || '还没有正文，点开继续记录。'}</p>
      <div className="note-meta"><span>{stamp(n.updatedAt)}</span>{n.tags?.slice(0,2).map(tag=><span className="list-tag" key={tag.id}>{tag.name}</span>)}{(!folder || n.folderId !== folderId) && <span>{library.folders.find(f => f.id === n.folderId)?.name}</span>}{n.source === 'agent' && <span className="agent-label"><Bot size={12}/>Agent 创建</span>}</div></div>
      {image && <img className="note-thumbnail" src={image} alt="" loading="lazy"/>}
    </button>;
  })}</div>;
  return <>
    <header className="library-header"><div>
      {folder && <button className="back-button" onClick={() => p.onFolder(folder.parentId)}><ArrowLeft size={18}/>{folder.parentId ? '上一级' : '文件夹'}</button>}
      <h1>{folder?.name || viewName}</h1><p className="header-subtitle">{folder ? `${notes.length} 篇笔记${folders.length ? ` · ${folders.length} 个子文件夹` : ''}` : '拾记 · 私人笔记库'}</p>
    </div><div className="header-controls"><button className="icon-button theme-button" aria-label={p.theme === 'light' ? '切换到深色模式' : '切换到浅色模式'} onClick={p.onTheme}>{p.theme === 'light' ? <Moon size={19}/> : <Sun size={19}/>}</button><button className="avatar" onClick={p.onSettings} aria-label="打开空间设置">{p.displayName?.slice(0,1)||'我'}</button></div></header>
    <label className="search-field"><Search size={18}/><input type="search" value={query} onChange={e => p.onQuery(e.target.value)} aria-label="搜索笔记" placeholder={folder ? '在这个文件夹中搜索' : '搜索笔记'}/></label>
    {view === 'trash' && <div className="inline-notice"><Trash2 size={15}/>回收站中的笔记可以打开后恢复。</div>}
    <div className="section-toolbar"><span className="section-caption">{query ? `搜索结果 · ${notes.length} 篇` : grouped ? folders.length ? folder ? '子文件夹' : '我的文件夹' : '笔记' : viewName}</span>
      {grouped ? <button className="icon-button add-folder-button" onClick={p.onAddFolder} aria-label="新建文件夹" title="新建文件夹"><FolderPlus size={21}/></button> : <span className="section-count">{query ? '' : `${notes.length} 篇`}</span>}
    </div>
    <section className="library-content" aria-label={grouped ? '文件夹与笔记' : '笔记列表'}>
      {p.searching ? <div className="empty-state" role="status">正在搜索…</div> : grouped ? <>
        {folders.length > 0 && <div className="folder-list">{folders.map(f => {
          const ids = descendants(library.folders, f.id);
          const count = library.notes.filter(n => !n.trashed && ids.includes(n.folderId)).length;
          const children = library.folders.filter(c => c.parentId === f.id);
          return <button className="folder-heading" key={f.id} onClick={() => p.onFolder(f.id)} aria-label={`${f.name}，${count} 篇笔记`}>
            <span className="folder-symbol">{f.isInbox || f.id === 'inbox' ? <Inbox size={23}/> : <FolderIcon size={23}/>}</span>
            <span className="folder-heading-copy"><strong>{f.name}</strong>{children.length > 0 && <span>{children.map(c => c.name).join('、')}</span>}</span>
            <span className="folder-count" aria-hidden="true">{count}</span><ChevronRight size={16}/>
          </button>;
        })}</div>}
        {direct.length > 0 && <>{folders.length > 0 && <h2 className="subsection-title">笔记</h2>}{noteList(direct)}</>}
        {!folders.length && !direct.length && <Empty title="这里还没有笔记" detail="记下第一条想法，文字和图片都可以。" action={p.onNew}/>}
      </> : notes.length ? noteList(notes) : <Empty title={query ? '没有找到相关笔记' : '这里还是空的'} detail={query ? '试试其他关键词，找回记录过的想法。' : '新的记录，会慢慢填满这里。'} action={query ? () => p.onQuery('') : view === 'trash' ? () => p.onFolder(null) : p.onNew} label={query ? '清除搜索' : view === 'trash' ? '返回文件夹' : '新建笔记'}/>}
    </section>
    {view !== 'trash' && (folders.length > 0 || notes.length > 0) && <button className="primary-button library-create" onClick={p.onNew}><Plus size={19}/>新建笔记</button>}
    {grouped && (folders.length > 0 || direct.length > 0) && <footer className="library-footer">{folders.length > 0 && `${folders.length} 个文件夹 · `}{notes.length} 篇笔记</footer>}
  </>;
}
function Empty({title,detail,action,label='新建笔记'}:{title:string;detail:string;action:()=>void;label?:string}) {
  return <div className="empty-state"><NotebookPen size={32} strokeWidth={1.4}/><h2>{title}</h2><p>{detail}</p><button className="secondary-button" onClick={action}>{label}</button></div>;
}
