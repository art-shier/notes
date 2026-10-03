import type {Folder, Note, Library, Tag} from './data';
export interface Account {id:string;email:string;display_name:string;role:string;csrf_token:string;quota_bytes:number;used_bytes:number}
export interface Token {id:string;name:string;prefix:string;scopes:string[];expires_at:string;revoked:boolean}
export interface Revision {version:number;title:string;excerpt:string;action:string;actor:'web'|'agent'|'system';saved_at:string;restored_from:number|null}
export interface RevisionDetail extends Revision {snapshot:Note}
export interface HistoryPage {items:Revision[];next_before_version:number|null;current_version:number;retention_limit:number}
export interface ExportOptions {include_trash:boolean;include_history:boolean}
export interface ExportBundle {id:string;download_url:string;ready:boolean;size_bytes:number;expires_at:string;counts:{notes:number;revisions:number;attachments:number}}
let csrfToken='';
export class ApiError extends Error {status:number;code:string;details:unknown;constructor(status:number,error:any){super(error?.message||'服务暂时不可用，请稍后重试。');this.status=status;this.code=error?.code||'request_failed';this.details=error?.details}}
async function request<T>(path:string,method='GET',body?:unknown,extraHeaders:Record<string,string>={}):Promise<T>{
  const headers:Record<string,string>={...extraHeaders};
  if(method!=='GET')headers['X-CSRF-Token']=csrfToken;
  if(body!==undefined&&!(body instanceof FormData))headers['Content-Type']='application/json';
  const retryCreate=method==='POST'&&path==='/notes'&&!!headers['Idempotency-Key'];
  let response:Response|undefined;
  for(let attempt=0;attempt<(retryCreate?2:1);attempt++){
    try{response=await fetch('/api/v1'+path,{method,headers,credentials:'same-origin',body:body instanceof FormData?body:body===undefined?undefined:JSON.stringify(body)})}
    catch(e){if(!retryCreate||attempt===1)throw e;await new Promise(resolve=>setTimeout(resolve,300));continue}
    if(retryCreate&&attempt===0&&[502,503,504].includes(response.status)){await new Promise(resolve=>setTimeout(resolve,300));continue}break;
  }
  if(!response)throw new Error('连接失败，请稍后重试。');
  if(response.status===204)return undefined as T;
  const data=await response.json().catch(()=>null);
  if(!response.ok)throw new ApiError(response.status,data?.error);
  return data as T;
}
export function toNote(r:any):Note{return {id:r.id,title:r.title,html:'',content:r.content_json,excerpt:r.excerpt,thumbnail:r.thumbnail,folderId:r.folder_id,favorite:r.favorite,source:r.source,updatedAt:r.updated_at,version:r.version,tags:r.tags||[],trashed:r.trashed}}
const toFolder=(f:any):Folder=>({id:f.id,name:f.name,parentId:f.parent_id,isInbox:f.is_inbox});
async function identify(promise:Promise<Account>){const account=await promise;csrfToken=account.csrf_token;return account}
export const api={
  search:async(query:string,folderId:string|null,view:string):Promise<Note[]>=>{const result:Note[]=[];let cursor:string|null=null;do{const params=new URLSearchParams({query,limit:'100',trash:String(view==='trash'),...(folderId?{folder_id:folderId}:{}),...(view==='favorites'?{favorite:'true'}:{}),...(cursor?{cursor}:{})});const page:{items:any[];next_cursor:string|null}=await request('/notes?'+params);result.push(...page.items.map(toNote));cursor=page.next_cursor}while(cursor);return view==='agent'?result.filter(n=>n.source==='agent'):result},
  me:()=>identify(request<Account>('/me')),
  login:(email:string,password:string)=>identify(request<Account>('/auth/login','POST',{email,password})),
  register:(email:string,password:string,display_name:string,invitation:string)=>identify(request<Account>('/auth/register','POST',{email,password,display_name,invitation})),
  logout:async()=>{await request('/auth/logout','POST');csrfToken=''},
  library:async():Promise<Library>=>{
    const [folders,tags]=await Promise.all([request<{items:any[]}>('/folders'),request<{items:Tag[]}>('/tags')]);const notes:Note[]=[];
    for(const trash of [false,true]){let cursor:string|null=null;do{const page:{items:any[];next_cursor:string|null}=await request('/notes?'+new URLSearchParams({limit:'100',trash:String(trash),...(cursor?{cursor}:{})}));notes.push(...page.items.map(toNote));cursor=page.next_cursor}while(cursor)}
    return {folders:folders.items.map(toFolder),notes,tags:tags.items};
  },
  note:async(id:string)=>toNote(await request('/notes/'+id)),
  createNote:async(folder_id:string)=>toNote(await request('/notes','POST',{folder_id},{'Idempotency-Key':crypto.randomUUID()})),
  patch:async(id:string,expected_version:number,patch:Record<string,unknown>)=>request<any>('/notes/'+id,'PATCH',{expected_version,...patch}),
  trash:async(n:Note)=>toNote(await request('/notes/'+n.id+(n.trashed?'/restore':'?expected_version='+n.version),n.trashed?'POST':'DELETE',n.trashed?{expected_version:n.version}:undefined)),
  folder:async(name:string,parent_id:string|null)=>toFolder(await request('/folders','POST',{name,parent_id})),
  upload:async(file:File)=>{const form=new FormData();form.append('file',file);return request<{id:string;url:string}>('/attachments','POST',form)},
  tokens:()=>request<{items:Token[]}>('/tokens'),
  createToken:(name:string,scopes:string[])=>request<Token & {secret:string}>('/tokens','POST',{name,scopes,expires_days:30}),
  revokeToken:(id:string)=>request('/tokens/'+id,'DELETE'),
  createTag:(name:string)=>request<Tag>('/tags','POST',{name}),
  history:(id:string,before?:number)=>request<HistoryPage>('/notes/'+id+'/history'+(before?'?before_version='+before:'')),
  revision:async(id:string,version:number):Promise<RevisionDetail>=>{const r=await request<any>('/notes/'+id+'/history/'+version);return {...r,snapshot:toNote(r.snapshot)}},
  restoreRevision:async(id:string,version:number,expected_version:number)=>toNote(await request('/notes/'+id+'/history/'+version+'/restore','POST',{expected_version})),
  prepareExport:(options:ExportOptions)=>request<ExportBundle>('/exports','POST',options),
  exports:()=>request<{items:ExportBundle[]}>('/exports'),
  downloadExport:(bundle:ExportBundle)=>{const a=document.createElement('a');a.href='/api/v1/exports/'+encodeURIComponent(bundle.id)+'/download';a.download='shiji-notes.zip';document.body.append(a);a.click();a.remove()}
};
