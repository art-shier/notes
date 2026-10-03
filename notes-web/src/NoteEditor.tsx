import { useEditor, EditorContent } from '@tiptap/react';
import {editorExtensions} from './editorExtensions';
import Placeholder from '@tiptap/extension-placeholder';
import { ArrowLeft, Star, MoreHorizontal, Bold, Italic, Heading2, List, ImagePlus, Quote, Undo2, Redo2, Trash2, RotateCcw, Check, Bot, History } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import type { Note, Folder, Tag } from './data';
import {NoteTags} from './NoteTags';
import {api} from './api';
import type {SaveState} from './saveQueue';
interface Props {note:Note;folders:Folder[];tags:Tag[];onCreateTag:(tag:Tag)=>void;onBack:()=>void;onHistory:()=>void;onChange:(patch:Partial<Note>)=>void;onTrash:()=>void;saveState:SaveState;disabled?:boolean;notify:(s:string)=>void;}
export function NoteEditor({note,folders,tags,onCreateTag,onBack,onHistory,onChange,onTrash,saveState,disabled,notify}:Props) {
  const [menu,setMenu]=useState(false);const file=useRef<HTMLInputElement>(null);
  const editor=useEditor({extensions:[...editorExtensions(),Placeholder.configure({placeholder:'写下你的想法，或插入一张图片…'})],content:note.content||note.html,editable:!note.trashed&&!disabled,onUpdate:({editor})=>onChange({content:editor.getJSON()})});
  useEffect(()=>{editor?.setEditable(!note.trashed&&!disabled,false)},[editor,note.trashed,disabled]);
  const [uploading,setUploading]=useState(false);
  const upload=async(f?:File)=>{if(!f||disabled||note.trashed)return;if(f.size>10*1024*1024){notify('图片不能超过 10 MiB。');return}setUploading(true);try{const a=await api.upload(f);editor?.chain().focus().insertContent({type:'image',attrs:{src:a.url,attachment_id:a.id,alt:f.name}}).run()}catch(e){notify((e as Error).message)}finally{setUploading(false)}};
  const status={saved:'已保存',pending:'待保存',saving:'保存中…',error:'保存失败',conflict:'保存冲突'}[saveState];
  return <section className="editor-page"><header className="editor-header"><button className="back-button" onClick={onBack}> <ArrowLeft size={18}/>返回</button><span className={`save-status ${saveState==='error'||saveState==='conflict'?'error':''}`} role="status">{saveState==='saved'&&<Check size={12}/>}{uploading?'图片上传中…':status}</span><div className="action-group"><button disabled={disabled} className={`icon-button ${note.favorite?'is-favorite':''}`} aria-pressed={note.favorite} aria-label={note.favorite?'取消收藏':'收藏笔记'} onClick={()=>onChange({favorite:!note.favorite})}><Star size={19}/></button><button disabled={disabled} className="icon-button" aria-label="更多笔记操作" aria-expanded={menu} onClick={()=>setMenu(!menu)}><MoreHorizontal size={21}/></button></div></header>
    {menu&&<div className="editor-menu"><button onClick={()=>{editor?.chain().focus().undo().run();setMenu(false)}} disabled={!editor?.can().undo()}><Undo2 size={16}/>撤销最近编辑</button><button onClick={()=>{setMenu(false);onHistory()}}><History size={16}/>历史版本</button><button className={note.trashed?'':'danger'} onClick={onTrash}>{note.trashed?<RotateCcw size={16}/>:<Trash2 size={16}/>} {note.trashed?'恢复笔记':'移入回收站'}</button></div>}
    {note.trashed&&<div className="inline-notice"><Trash2 size={15}/>这篇笔记在回收站中，恢复后可以继续编辑。</div>}
    <input className="note-title-input" aria-label="笔记标题" placeholder="写个标题…" value={note.title} maxLength={300} disabled={note.trashed||disabled} onChange={e=>onChange({title:e.target.value})}/>
    <div className="editor-meta"><label className="folder-select-label"><span className="sr-only">笔记文件夹</span><select value={note.folderId} disabled={note.trashed||disabled} onChange={e=>onChange({folderId:e.target.value})}>{folders.map(f=><option key={f.id} value={f.id}>{f.name}</option>)}</select></label><span>私人笔记</span>{note.source==='agent'&&<span className="agent-label"><Bot size={13}/>Agent 创建</span>}</div>
    <NoteTags selected={note.tags||[]} tags={tags} disabled={note.trashed||disabled} onChange={tags=>onChange({tags})} onCreate={onCreateTag}/>
    {!note.trashed&&!disabled&&<div className="format-toolbar" aria-label="正文格式"><button aria-label="加粗" aria-pressed={editor?.isActive('bold')} onClick={()=>editor?.chain().focus().toggleBold().run()}><Bold size={17}/></button><button aria-label="斜体" aria-pressed={editor?.isActive('italic')} onClick={()=>editor?.chain().focus().toggleItalic().run()}><Italic size={17}/></button><button aria-label="小标题" aria-pressed={editor?.isActive('heading',{level:2})} onClick={()=>editor?.chain().focus().toggleHeading({level:2}).run()}><Heading2 size={19}/></button><span className="toolbar-divider"/><button aria-label="无序列表" aria-pressed={editor?.isActive('bulletList')} onClick={()=>editor?.chain().focus().toggleBulletList().run()}><List size={19}/></button><button aria-label="引用" aria-pressed={editor?.isActive('blockquote')} onClick={()=>editor?.chain().focus().toggleBlockquote().run()}><Quote size={17}/></button><button aria-label="插入图片" disabled={uploading} onClick={()=>file.current?.click()}><ImagePlus size={19}/></button><button aria-label="重做" disabled={!editor?.can().redo()} onClick={()=>editor?.chain().focus().redo().run()}><Redo2 size={17}/></button></div>}
    <EditorContent editor={editor} className="document-body" onPaste={e=>{const image=[...e.clipboardData.files].find(f=>f.type.startsWith('image/'));if(image){e.preventDefault();void upload(image)}}}/><input ref={file} type="file" accept="image/jpeg,image/png,image/webp,image/gif" hidden onChange={e=>{void upload(e.target.files?.[0]);e.target.value=''}}/>
    <footer className="editor-footer"><span>{editor?.getText().replace(/\s/g,'').length||0} 字</span><span>私人空间 · 服务端存储</span></footer>
  </section>
}
