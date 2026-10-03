import {useState} from 'react';
import {Tag as TagIcon,Plus,X,Check} from 'lucide-react';
import {api} from './api';
import type {Tag} from './data';

export function NoteTags({selected,tags,disabled,onChange,onCreate}:{selected:Tag[];tags:Tag[];disabled?:boolean;onChange:(tags:Tag[])=>void;onCreate:(tag:Tag)=>void}){
  const [name,setName]=useState('');const [busy,setBusy]=useState(false);const [error,setError]=useState('');
  const toggle=(tag:Tag)=>onChange(selected.some(t=>t.id===tag.id)?selected.filter(t=>t.id!==tag.id):[...selected,tag]);
  return <div className="note-tags" aria-label="笔记标签">
    <div className="tag-chips">{selected.map(tag=><span className="tag-chip" key={tag.id}><TagIcon size={12}/>{tag.name}{!disabled&&<button disabled={busy} aria-label={`移除标签 ${tag.name}`} onClick={()=>toggle(tag)}><X size={13}/></button>}</span>)}</div>
    {!disabled&&<details className="tag-picker"><summary><Plus size={14}/>{selected.length?'编辑标签':'添加标签'}</summary><div className="tag-picker-panel">
      <p className="section-caption">选择标签</p><fieldset disabled={busy}><div className="tag-options">{tags.length?tags.map(tag=><button className="tag-option" key={tag.id} aria-pressed={selected.some(t=>t.id===tag.id)} onClick={()=>toggle(tag)}><TagIcon size={14}/><span>{tag.name}</span>{selected.some(t=>t.id===tag.id)&&<Check size={15}/>}</button>):<p className="settings-copy">还没有标签，可以先创建一个。</p>}</div>
      <form className="tag-create" onSubmit={async e=>{e.preventDefault();if(!name.trim()||busy)return;setBusy(true);setError('');try{const tag=await api.createTag(name.trim());onCreate(tag);onChange([...selected,tag]);setName('')}catch(e){setError((e as Error).message)}finally{setBusy(false)}}}><label className="sr-only" htmlFor="new-tag-name">新标签名称</label><input id="new-tag-name" placeholder="新标签名称" maxLength={40} required value={name} onChange={e=>{setName(e.target.value);setError('')}}/><button className="secondary-button" type="submit" disabled={!name.trim()}>{busy?'创建中…':'创建'}</button></form>
      </fieldset>{error&&<p className="field-error" role="alert">{error}</p>}
    </div></details>}
  </div>;
}
