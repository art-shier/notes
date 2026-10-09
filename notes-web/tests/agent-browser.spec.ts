// Real, disposable Notes process + online installer + CLI + browser approval.
import {test,expect,request} from '@playwright/test';
import {spawn,spawnSync,type ChildProcess} from 'node:child_process';
import {mkdtempSync,mkdirSync,readFileSync,rmSync,existsSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {resolve,join} from 'node:path';
import {createServer} from 'node:net';

test('Agent installs, opens Notes, waits for user approval and remembers a scoped credential',async({page})=>{
 const repo=resolve('..');const temporary=mkdtempSync(join(tmpdir(),'notes-agent-e2e-'));
 const binary=join(temporary,process.platform==='win32'?'shiji.exe':'shiji');const clientHome=join(temporary,'client-home');mkdirSync(clientHome);
 const python=process.env.NOTES_TEST_PYTHON||(process.platform==='win32'?'python':'python3');
 const socket=createServer();await new Promise<void>(r=>socket.listen(0,'127.0.0.1',r));const port=(socket.address() as {port:number}).port;await new Promise<void>(r=>socket.close(()=>r()));
 const origin=`http://127.0.0.1:${port}`;
 const env={...process.env,DATABASE_URL:'sqlite:///'+join(temporary,'notes.db').replaceAll('\\','/'),APP_ORIGIN:origin,COOKIE_SECURE:'false',REQUIRE_DATABASE_URL:'false',LISTEN_ADDR:`127.0.0.1:${port}`,WEB_DIR:join(repo,'notes-web/dist'),ATTACHMENTS_DIR:join(temporary,'attachments'),EXPORTS_DIR:join(temporary,'exports')};
 let server:ChildProcess|undefined;const logins:ChildProcess[]=[];
 const command=(args:string[])=>spawnSync(binary,args,{env,encoding:'utf8'});
 const cliEnv={...env,PYTHONIOENCODING:'utf-8',NOTES_ALLOW_LOCAL_HTTP:'1',NOTES_API_URL:'',NOTES_API_TOKEN:''};
 const script=join(temporary,'skills/shiji-notes/scripts');
 const wrapper='import sys; from pathlib import Path; from unittest.mock import patch; sys.path.insert(0,sys.argv[1]); import notes; p=patch.object(Path,"home",return_value=Path(sys.argv[2])); p.start(); raise SystemExit(notes.main(sys.argv[3:]))';
 const cli=(args:string[])=>spawnSync(python,['-c',wrapper,script,clientHome,...args],{env:cliEnv,encoding:'utf8'});
 async function login(replace=false){
  const child=spawn(python,['-c',wrapper,script,clientHome,'login','--server',origin,'--no-browser','--name','Browser test Agent',...(replace?['--replace']:[])],{env:cliEnv});logins.push(child);
  let output='',errors='';child.stdout!.on('data',b=>output+=b.toString());
  const completion=new Promise<number|null>(r=>child.on('exit',r));
  const link=await new Promise<string>((resolve,reject)=>{
   const timer=setTimeout(()=>reject(new Error('CLI did not produce an authorization link')),15000);
   child.stderr!.on('data',b=>{errors+=b.toString();for(const line of errors.split('\n')){try{const e=JSON.parse(line);if(e.event==='authorization_required'){clearTimeout(timer);resolve(e.verification_uri)}}catch{}}});
   child.on('exit',()=>{clearTimeout(timer);reject(new Error('CLI exited before user authorization'))});
  });
  return {link,child,completion,output:()=>output,errors:()=>errors};
 }
 try{
  const built=spawnSync('go',['build','-o',binary,'./cmd/shiji'],{cwd:join(repo,'notes-server-go'),encoding:'utf8'});expect(built.status,built.stderr).toBe(0);
  expect(command(['migrate']).status).toBe(0);
  const bootstrap=command(['bootstrap','--email','agent-test@example.test']);expect(bootstrap.status).toBe(0);const invitation=new URL(bootstrap.stdout.trim()).searchParams.get('invite');
  server=spawn(binary,['serve'],{env,stdio:'ignore'});
  const http=await request.newContext();await expect.poll(async()=>{try{return (await http.get(origin+'/api/v1/health/ready')).status()}catch{return 0}},{timeout:10000}).toBe(200);
  const password='disposable-test-password-123';expect((await http.post(origin+'/api/v1/auth/register',{headers:{Origin:origin},data:{email:'agent-test@example.test',password,display_name:'Agent 测试',invitation}})).status()).toBe(201);
  const install=spawnSync(python,[join(repo,'install-client.py'),'--server',origin,'--skills-dir',join(temporary,'skills'),'--bin-dir',join(temporary,'bin')],{env:cliEnv,encoding:'utf8'});expect(install.status,install.stderr).toBe(0);
  expect(existsSync(join(script,'agent_auth.py'))).toBe(true);
  const connecting=await login();await page.goto(connecting.link);
  await expect(page.getByText('请先登录 Notes，再核对并批准 Agent 的连接请求。')).toBeVisible();
  await page.getByLabel('邮箱').fill('agent-test@example.test');await page.getByLabel('密码',{exact:true}).fill(password);await page.getByRole('button',{name:'登录',exact:true}).click();
  await expect(page.getByRole('heading',{name:'授权 Agent 连接'})).toBeVisible();expect(new URL(page.url()).hash).toContain('agent-authorize');
  await expect(page.getByRole('button',{name:'允许授权'})).toBeDisabled();
  await page.getByLabel('授权码与我的 Agent 显示的一致').check();
  await page.screenshot({path:'test-results/agent-approval-mobile.png',fullPage:true});
  await page.getByRole('button',{name:'允许授权'}).click();await expect(page.getByRole('heading',{name:'已允许连接'})).toBeVisible();
  expect(await connecting.completion).toBe(0);expect(JSON.parse(connecting.output()).status).toBe('connected');
  const profile=JSON.parse(readFileSync(join(clientHome,'.shiji-notes/client.json'),'utf8'));
  expect(connecting.output()).not.toContain(profile.token);expect(connecting.errors()).not.toContain(profile.token);
  const identity=cli(['whoami']);expect(identity.status,identity.stderr).toBe(0);expect(JSON.parse(identity.stdout).account.email).toBe('agent-test@example.test');
  expect(cli(['folders','list']).status).toBe(0);const deniedWrite=cli(['folders','create','Should not exist']);expect(deniedWrite.status).toBe(1);expect(JSON.parse(deniedWrite.stderr).error.code).toBe('scope_denied');
  await page.getByRole('button',{name:'返回我的笔记'}).click();await page.getByRole('button',{name:'打开空间设置'}).click();await page.getByRole('button',{name:'Agent 接入'}).click();
  await expect(page.getByRole('heading',{name:'让 Agent 连接你的笔记'})).toBeVisible();
  await expect(page.getByRole('link',{name:/阅读 Skill 文本/})).toHaveAttribute('href','/agent/SKILL.md');
  await expect(page.getByRole('link',{name:/阅读 API 合约/})).toHaveAttribute('href','/agent/references/api.md');
  await expect(page.getByRole('link',{name:/下载 Python 辅助包（可选）/})).toHaveAttribute('href','/agent/shiji-notes.zip');
  expect(await page.getByLabel('给 Agent 的安装与授权说明').inputValue()).toContain(origin+'/agent/SKILL.md');
  await page.screenshot({path:'test-results/agent-entry-mobile.png',fullPage:true});
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
  await page.setViewportSize({width:1280,height:900});await page.screenshot({path:'test-results/agent-entry-desktop.png',fullPage:true});
  const second=await login(true);await page.goto(second.link);await page.getByLabel('访问权限').selectOption('write');await page.getByLabel('授权码与我的 Agent 显示的一致').check();await page.getByRole('button',{name:'允许授权'}).click();expect(await second.completion).toBe(0);
  expect(cli(['folders','create','Agent created folder']).status).toBe(0);
  expect(cli(['logout']).status).toBe(0);expect(existsSync(join(clientHome,'.shiji-notes/client.json'))).toBe(false);
  const denied=await login();await page.goto(denied.link);await expect(page.getByRole('button',{name:'允许授权'})).toBeDisabled();await expect(page.getByLabel('访问权限')).toHaveValue('read');await page.getByRole('button',{name:'拒绝连接'}).click();expect(await denied.completion).toBe(1);expect(denied.errors()).toContain('authorization_denied');expect(existsSync(join(clientHome,'.shiji-notes/client.json'))).toBe(false);
  await http.dispose();
 }finally{
  for(const child of logins)if(child.exitCode===null)child.kill();
  if(server){const done=new Promise<void>(r=>server!.once('exit',()=>r()));server.kill();await done}
  rmSync(temporary,{recursive:true,force:true});
 }
});
