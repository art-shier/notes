export function authorizationCode(hash:string):string|null{
 if(!/^#agent-authorize(?:\?|$)/.test(hash))return null;
 const code=new URLSearchParams(hash.split('?')[1]||'').get('code')?.toUpperCase()||'';
 return /^[A-Z2-7]{4}-[A-Z2-7]{4}$/.test(code)?code:'';
}

export function agentInstruction(origin:string):string{
 return `请为我安装拾记 shiji-notes Skill，并连接我的 Notes：${origin}

1. 阅读在线 Skill：${origin}/agent/SKILL.md
2. 阅读 API 合约：${origin}/agent/references/api.md
   将两份文本保存为你实际 skills 目录下的 shiji-notes/SKILL.md 和 references/api.md。优先直接请求 ${origin}/api/v1，无需先安装 CLI 或下载 ZIP。
3. 检查已有私有连接。首次连接按 Skill 的设备授权协议发起请求，在 Notes App / 浏览器新标签页打开同源授权链接，提示我核对授权码、账户、权限和有效期，由我亲自确认。你不能代我点击允许授权或提交审批。
4. 授权后核验账户，将凭据保存在当前用户私有的 ~/.shiji-notes/client.json，后续自动读取。不要将完整 Token 或 device_code 写入聊天、模型记忆、仓库或日志；不要用会输出凭据的请求工具。
5. 用 GET /auth/agent/me 和 GET /folders 验证连接，只汇报连接状态和授权范围。
6. 如果当前工具无法直接发起请求，可以使用可选 Python 辅助工具：下载并检查 ${origin}/agent/install-client.py，使用 Python 3.10+ 执行 python install-client.py --server "${origin}"，按实际 skills 目录添加 --skills-dir。
   然后执行 shiji-notes login --server "${origin}" --no-browser --name "我的 Agent"，保持运行，并打开输出的授权链接等待我确认。完整辅助包：${origin}/agent/shiji-notes.zip。
7. 请求使用明确的 ShijiNotes User-Agent（见 Skill）。如果收到 Cloudflare HTML 403 / Error 1010，应报告边缘拦截；Python 辅助工具也需要服务允许 API 请求，不能将其当成权限不足或反复登录。`;
}
