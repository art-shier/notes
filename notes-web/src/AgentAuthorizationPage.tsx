import {useEffect,useState} from 'react';
import {ArrowLeft,Bot,Check,ShieldCheck,X} from 'lucide-react';
import {api,type Account,type AgentRequest} from './api';

export function AgentAuthorizationPage({account,code}:{account:Account;code:string}){
 const [grant,setGrant]=useState<AgentRequest|null>(null);const [error,setError]=useState('');const [busy,setBusy]=useState(false);const [access,setAccess]=useState('read');const [trash,setTrash]=useState(false);const [days,setDays]=useState('30');const [verified,setVerified]=useState(false);const [remaining,setRemaining]=useState(0);
 async function load(clearError=true){if(clearError)setError('');try{setGrant(await api.agentRequest(code))}catch(e){setError((e as Error).message)}}
 useEffect(()=>{if(code)void load();else setError('授权链接不完整，请让 Agent 重新发起连接。')},[code]);
 useEffect(()=>{if(!grant)return;const update=()=>setRemaining(Math.max(0,Math.ceil((Date.parse(grant.expires_at)-Date.now())/1000)));update();const t=setInterval(update,1000);return()=>clearInterval(t)},[grant]);
 const status=grant?.status==='pending'&&Date.parse(grant.expires_at)<=Date.now()?'expired':grant?.status;
 async function decide(approve:boolean){if(busy)return;setBusy(true);setError('');try{const result=await api.agentDecision(code,{approve,access,allow_trash:access==='write'&&trash,expires_days:Number(days)});setGrant(g=>g?{...g,status:result.status}:g)}catch(e){setError((e as Error).message);void load(false)}finally{setBusy(false)}}
 const back=()=>{location.hash='';location.reload()};
 return <main className="app-shell"><section className="app-content agent-authorization"><header><button className="back-button" onClick={back} disabled={busy}><ArrowLeft size={18}/>返回 Notes</button><span className="agent-symbol"><Bot size={27}/></span><h1>授权 Agent 连接</h1><p className="header-subtitle">请核对请求，再选择允许的权限。</p></header>
  <div className="authorization-account"><span className="avatar">{account.display_name.slice(0,1)}</span><div><strong>{account.display_name}的私人空间</strong><span>{account.email}</span></div></div>
  {error&&<div className="inline-notice" role="alert">{error}</div>}
  {!grant&&!error&&<p className="settings-copy" role="status">正在读取授权请求…</p>}
  {!grant&&error&&code&&<button className="secondary-button full-width" onClick={()=>void load()}>重新读取</button>}
  {grant&&<><div className="authorization-request"><span>请求连接的 Agent</span><strong>{grant.name}</strong><span>请与 Agent 显示的授权码核对</span><code>{grant.user_code}</code></div>
  {status==='pending'?<form onSubmit={e=>{e.preventDefault();void decide(true)}}><fieldset disabled={busy}><label className="form-field">访问权限<select value={access} onChange={e=>{setAccess(e.target.value);setTrash(false)}}><option value="read">只读：查找与读取</option><option value="write">读写：读取、创建与修改</option></select><small>{access==='read'?'可读取此账户的全部笔记、文件夹、标签和图片。':'可读取此账户的全部内容，创建或修改笔记、文件夹、标签和图片。'}</small></label>{access==='write'&&<label className="token-extra-scope"><input type="checkbox" checked={trash} onChange={e=>setTrash(e.target.checked)}/>同时允许移入回收站和恢复</label>}<label className="form-field">凭据有效期<select value={days} onChange={e=>setDays(e.target.value)}><option value="7">7 天</option><option value="30">30 天</option><option value="90">90 天</option><option value="365">365 天</option></select></label><label className="token-extra-scope authorization-verify"><input type="checkbox" checked={verified} onChange={e=>setVerified(e.target.checked)}/>授权码与我的 Agent 显示的一致</label><p className="settings-copy">此请求还有 {Math.floor(remaining/60)} 分 {remaining%60} 秒有效。请仅批准你主动发起的连接。</p><button className="primary-button full-width" disabled={!verified||busy}>{busy?'正在处理…':'允许授权'}</button><button type="button" className="secondary-button full-width" disabled={busy} onClick={()=>void decide(false)}>拒绝连接</button></fieldset></form>:<div className="authorization-result" role="status">{status==='approved'?<Check size={29}/>:<X size={29}/>}<h2>{status==='approved'?'已允许连接':status==='denied'?'已拒绝连接':status==='expired'?'授权请求已过期':'连接已取消'}</h2><p>{status==='approved'?'返回 Agent 等待连接完成。凭据由 Agent 本地保存，你可以在空间设置 → Agent 接入中撤销。':'这次请求已结束。如需连接，请让 Agent 重新发起授权。'}</p><button className="secondary-button full-width" onClick={back}>返回我的笔记</button></div>}</>}
  <div className="privacy-note"><ShieldCheck size={16}/><p>授权仅对当前账户生效，Agent 无法访问其他用户的空间。浏览器不会显示或传递完整 Token。</p></div>
 </section></main>;
}
