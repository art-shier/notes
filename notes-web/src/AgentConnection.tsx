import {useEffect,useRef,useState} from 'react';
import {Bot,Check,Copy,Download,ExternalLink,ShieldCheck} from 'lucide-react';
import {api} from './api';
import {agentInstruction} from './agentHandoff';

export function AgentConnection({notify}:{notify:(message:string)=>void}){
 const [copied,setCopied]=useState(false);const [version,setVersion]=useState('');const [error,setError]=useState('');
 const instruction=agentInstruction(location.origin);const text=useRef<HTMLTextAreaElement>(null);
 useEffect(()=>{let live=true;void api.agentAccess().then(info=>{if(live)setVersion(info.version)}).catch(()=>{if(live)setError('在线安装资源暂时不可用，请稍后刷新重试。')});return()=>{live=false}},[]);
 useEffect(()=>{if(!copied)return;const timeout=setTimeout(()=>setCopied(false),3000);return()=>clearTimeout(timeout)},[copied]);
 async function copy(){try{await navigator.clipboard.writeText(instruction);setCopied(true);notify('接入说明已复制，可以发给你的 Agent。')}catch{text.current?.focus();text.current?.select();notify('请复制已选中的接入说明。')}}
 return <div className="agent-connection">
  <div className="agent-intro"><span className="agent-symbol"><Bot size={25}/></span><div><h2>让 Agent 连接你的笔记</h2><p>在 App 中确认授权，之后自动记住连接。</p></div></div>
  <ol className="agent-steps"><li><span>1</span><div><strong>把接入说明发给 Agent</strong><p>Agent 阅读 Skill，通过 API 发起连接。</p></div></li><li><span>2</span><div><strong>打开 Notes，亲自确认授权</strong><p>Agent 会打开授权页。核对授权码和账户，选择权限与有效期。</p></div></li><li><span>3</span><div><strong>连接完成，后续直接使用</strong><p>凭据保存在 Agent 本地；可以随时在这里撤销。</p></div></li></ol>
  <div className="agent-handoff"><div className="agent-handoff-heading"><h3>复制给 Agent</h3><span>安装 + 授权 + 验证</span></div><p>直接发送下面的完整说明，由 Agent 完成安装和连接。</p><textarea ref={text} readOnly value={instruction} aria-label="给 Agent 的安装与授权说明"/><button className="primary-button full-width" onClick={()=>void copy()}>{copied?<Check size={17}/>:<Copy size={17}/>} {copied?'已复制接入说明':'复制完整接入说明'}</button></div>
  <div className="agent-resources"><div><h3>在线 Skill 资源</h3>{version&&<span>v{version}</span>}</div><a href="/agent/SKILL.md" target="_blank" rel="noopener noreferrer"><div><strong>阅读 Skill 文本</strong><small>{location.origin}/agent/SKILL.md</small></div><ExternalLink size={17}/></a><a href="/agent/references/api.md" target="_blank" rel="noopener noreferrer"><div><strong>阅读 API 合约</strong><small>授权流程与笔记接口 · 支持直接 HTTP 请求</small></div><ExternalLink size={17}/></a><a href="/agent/shiji-notes.zip"><div><strong>下载 Python 辅助包（可选）</strong><small>无法直接请求时使用 · 包含 Skill 和 API 合约</small></div><Download size={17}/></a><a href="/agent/install-client.py" target="_blank" rel="noopener noreferrer"><div><strong>查看 Python 安装器（可选）</strong><small>Python 3.10+ · 辅助授权与凭据保存</small></div><ExternalLink size={17}/></a>{error&&<p className="field-error" role="alert">{error}</p>}</div>
  <div className="privacy-note"><ShieldCheck size={16}/><p>只有你可以批准授权。完整凭据保存在 Agent 当前用户的私有配置文件中，不需要复制到聊天里。</p></div>
 </div>;
}
