import type {JSONContent} from '@tiptap/core';
export interface Folder { id: string; name: string; parentId: string | null; isInbox?:boolean; }
export interface Tag { id:string; name:string; }
export interface Note { id: string; title: string; html: string; content?:JSONContent; excerpt?:string; thumbnail?:string; version?:number; tags?:Tag[]; folderId: string; favorite: boolean; source: 'web' | 'agent'; updatedAt: string; trashed?: boolean; }
export interface Library { folders: Folder[]; notes: Note[]; tags?:Tag[]; }
export const seed: Library = {
  folders: [{id:'inbox',name:'收件箱',parentId:null},{id:'work',name:'工作',parentId:null},{id:'design',name:'产品设计',parentId:'work'},{id:'tech',name:'技术积累',parentId:'work'},{id:'life',name:'生活',parentId:null},{id:'ideas',name:'灵感',parentId:null}],
  notes: [
    {id:'walk',title:'周末去山里走走',html:'<p>想找一条安静的路线，带上相机，把这一周的忙碌暂时放下。</p><img src="/route.svg" alt="从山脚经过树林到山顶的周末路线草图"><h2>出发前准备</h2><ul><li>提前看天气</li><li>水和简单的食物</li><li>相机与充电宝</li></ul><p>不用急着走到山顶，沿途也是风景。</p>',folderId:'life',favorite:true,source:'web',updatedAt:'2026-10-01T02:24:00Z'},
    {id:'home',title:'给想法一个安放的地方',html:'<p>工作资料、生活片段和突然出现的灵感，都可以放在这里。</p><h2>记录要足够轻松</h2><p>打开就能写，图片直接插入，再慢慢整理分类。</p><blockquote><p>先把每个想法收好，之后再把它们连接起来。</p></blockquote>',folderId:'design',favorite:false,source:'web',updatedAt:'2026-10-01T01:18:00Z'},
    {id:'week',title:'整理本周的产品灵感',html:'<p>已把本周的想法整理到灵感分类。</p><h2>值得继续探索的方向</h2><ul><li>自然语言记录</li><li>图文混排</li><li>按主题找回</li></ul>',folderId:'ideas',favorite:false,source:'agent',updatedAt:'2026-09-30T12:36:00Z'},
    {id:'idea',title:'一个还没展开的小点子',html:'<p>想到什么先记下来，不一定要马上整理。</p>',folderId:'inbox',favorite:false,source:'web',updatedAt:'2026-09-30T08:42:00Z'},
    {id:'editor',title:'编辑器体验观察',html:'<h2>需要关注的细节</h2><p>图片粘贴是否顺畅？保存状态是否明确？手机编辑是否方便？</p>',folderId:'tech',favorite:true,source:'web',updatedAt:'2026-09-29T03:00:00Z'}
  ]
};
export const plainText = (html: string) => {
  const element = document.createElement('div');
  element.innerHTML = html;
  element.querySelectorAll('p,h1,h2,h3,h4,h5,h6,li,blockquote,pre,br').forEach(block => block.after(document.createTextNode(' ')));
  return (element.textContent || '').replace(/\s+/g, ' ').trim();
};
export const stamp = (iso: string) => new Intl.DateTimeFormat('zh-CN',{month:'numeric',day:'numeric',timeZone:'Asia/Shanghai'}).format(new Date(iso));
export function descendants(folders: Folder[], id: string): string[] { return [id,...folders.filter(f=>f.parentId===id).flatMap(f=>descendants(folders,f.id))]; }
export function readLibrary(): Library {
  try { const saved = localStorage.getItem('shiji.library.v1'); if(saved) { const data = JSON.parse(saved); if(Array.isArray(data.folders)&&Array.isArray(data.notes)&&data.folders.some((f:Folder)=>f.id==='inbox')&&data.folders.every((f:Folder)=>typeof f.id==='string'&&typeof f.name==='string')&&data.notes.every((n:Note)=>typeof n.id==='string'&&typeof n.html==='string'&&typeof n.title==='string')) return data; } } catch { /* start from sample data */ }
  return structuredClone(seed);
}
