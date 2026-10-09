import {useState} from 'react';
import {NotebookPen} from 'lucide-react';
import {api,type Account} from './api';
export function AuthPage({onLogin}:{onLogin:(account:Account)=>void}){
  const params=new URLSearchParams(location.search);const invitation=params.get('invite')||'';
  const [email,setEmail]=useState(params.get('email')||'');const [password,setPassword]=useState('');const [name,setName]=useState('');const [error,setError]=useState('');const [busy,setBusy]=useState(false);
  return <section className="auth-page"><span className="auth-symbol"><NotebookPen size={28}/></span><h1>拾记</h1><p className="header-subtitle">给每一个想法，留一个位置。</p><h2>{invitation?'创建你的笔记空间':'登录你的笔记空间'}</h2>
    <form onSubmit={async e=>{e.preventDefault();setBusy(true);setError('');try{const account=invitation?await api.register(email,password,name,invitation):await api.login(email,password);history.replaceState(null,'',location.pathname+location.hash);onLogin(account)}catch(e){setError((e as Error).message)}finally{setBusy(false)}}}>
    {invitation&&<label className="form-field">显示名称<input autoComplete="nickname" required maxLength={60} value={name} onChange={e=>setName(e.target.value)}/></label>}
    <label className="form-field">邮箱<input type="email" autoComplete="username" required maxLength={254} value={email} onChange={e=>setEmail(e.target.value)}/></label>
    <label className="form-field">密码<input aria-label="密码" type="password" autoComplete={invitation?'new-password':'current-password'} required minLength={invitation?12:1} maxLength={256} value={password} onChange={e=>setPassword(e.target.value)}/>{invitation&&<small>至少 12 个字符。</small>}</label>
    {error&&<p className="field-error" role="alert">{error}</p>}<button className="primary-button full-width" disabled={busy}>{busy?'正在处理…':invitation?'创建空间':'登录'}</button></form>
    <p className="auth-help">{invitation?'邀请有效期为 24 小时，使用一次后失效。':'当前采用邀请制。新账户请使用管理员提供的邀请链接。'}</p>
  </section>;
}
