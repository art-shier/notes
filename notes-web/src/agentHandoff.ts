export function authorizationCode(hash:string):string|null{
 if(!/^#agent-authorize(?:\?|$)/.test(hash))return null;
 const code=new URLSearchParams(hash.split('?')[1]||'').get('code')?.toUpperCase()||'';
 return /^[A-Z2-7]{4}-[A-Z2-7]{4}$/.test(code)?code:'';
}

export function agentInstruction(origin:string):string{
 return `请为我安装拾记 shiji-notes Skill，并连接我的 Notes：${origin}

1. 阅读在线 Skill：${origin}/agent/SKILL.md
2. 下载并检查同源安装器：${origin}/agent/install-client.py
   使用 Python 3.10+ 执行：python install-client.py --server "${origin}"
   根据你实际使用的 skills 目录添加 --skills-dir。要安装完整包，不能只保存 SKILL.md；完整包地址：${origin}/agent/shiji-notes.zip
3. 先运行 shiji-notes whoami 检查已有连接。若尚未连接，执行：shiji-notes login --server "${origin}" --no-browser --name "我的 Agent"
   若命令不在 PATH，用已安装 Skill 中的 python scripts/notes.py 执行相同命令。保持 login 运行。
4. 将命令输出的授权链接在 Notes App / 浏览器新标签页打开，提示我核对授权码、账户、权限和有效期，由我亲自确认。你不能代我点击允许授权或提交审批。
5. 授权后让 CLI 将凭据保存在当前用户的 ~/.shiji-notes/client.json，后续自动读取。不要将完整 Token 写入聊天、模型记忆、仓库或日志。
6. 用 whoami 和 folders list 验证连接，只汇报连接状态和授权范围。`;
}
