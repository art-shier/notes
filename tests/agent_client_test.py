"""Credential behavior tests, entirely within disposable directories."""
import importlib.util, json, os, sys, tempfile, unittest, subprocess
from http.server import BaseHTTPRequestHandler,HTTPServer
from threading import Thread
from pathlib import Path
from unittest.mock import patch
ROOT=Path(__file__).resolve().parents[1]
sys.path.insert(0,str(ROOT/'notes-skill/shiji-notes/scripts'))
import notes

class HttpCompatibility(unittest.TestCase):
    def test_installer_uses_the_same_explicit_identity_for_downloads(self):
        spec=importlib.util.spec_from_file_location('installer',ROOT/'install-client.py')
        installer=importlib.util.module_from_spec(spec);spec.loader.exec_module(installer)
        spec=importlib.util.spec_from_file_location('assets',ROOT/'scripts/build_agent_assets.py')
        assets=importlib.util.module_from_spec(spec);spec.loader.exec_module(assets)
        resources=assets.resources();received=[]
        class Handler(BaseHTTPRequestHandler):
            def do_GET(self):
                received.append(self.headers.get('User-Agent'))
                body=(json.dumps({'sha256':installer.digest(resources['shiji-notes.zip'])}).encode() if self.path=='/api/v1/agent-access' else resources['shiji-notes.zip'])
                self.send_response(200);self.end_headers();self.wfile.write(body)
            def log_message(self,*args):pass
        server=HTTPServer(('127.0.0.1',0),Handler);worker=Thread(target=server.serve_forever,daemon=True);worker.start()
        try:
            origin=f'http://127.0.0.1:{server.server_port}'
            installer.fetch(origin+'/download')
            args=type('Args',(),{'server':origin,'source_dir':None})()
            with patch.dict(os.environ,{'NOTES_ALLOW_LOCAL_HTTP':'1'}):files,_=installer.bundle(args)
            self.assertIn('scripts/notes.py',files)
            self.assertEqual(len(received),3)
            self.assertTrue(all(ua.startswith('ShijiNotes/') for ua in received))
        finally:server.shutdown();server.server_close();worker.join()

    def test_explicit_client_identity_and_cloudflare_errors(self):
        received=[]
        class Handler(BaseHTTPRequestHandler):
            def do_GET(self):
                received.append(self.headers.get('User-Agent'))
                if self.path.endswith('/edge'):
                    self.send_response(403);self.send_header('CF-Ray','fixture-ray');body=b'<html>Cloudflare Error 1010: Access denied</html>'
                elif self.path.endswith('/denied'):
                    self.send_response(403);body=b'{"error":{"code":"scope_denied","message":"Missing notes:read"}}'
                elif self.headers.get('User-Agent','').startswith('Python-urllib/'):
                    self.send_response(403);self.send_header('CF-Ray','fixture-ray');body=b'<html>Cloudflare Error 1010: Access denied</html>'
                else:
                    self.send_response(200);body=b'{"items":[]}'
                self.end_headers();self.wfile.write(body)
            def log_message(self,*args):pass
        server=HTTPServer(('127.0.0.1',0),Handler);worker=Thread(target=server.serve_forever,daemon=True);worker.start()
        try:
            client=notes.Client({'NOTES_API_URL':f'http://127.0.0.1:{server.server_port}/api/v1','NOTES_API_TOKEN':'fixture','NOTES_ALLOW_LOCAL_HTTP':'1'})
            self.assertEqual(client.request('GET','/notes'),{'items':[]})
            self.assertTrue(received[-1].startswith('ShijiNotes/'))
            with self.assertRaises(notes.CliError) as blocked:client.request('GET','/edge')
            self.assertEqual(blocked.exception.code,'cloudflare_blocked')
            self.assertIn('1010',str(blocked.exception))
            self.assertIn('User-Agent',str(blocked.exception))
            self.assertNotIn('<html>',str(blocked.exception))
            with self.assertRaises(notes.CliError) as denied:client.request('GET','/denied')
            self.assertEqual(denied.exception.code,'scope_denied')
            self.assertEqual(received.count(received[-1]),3,'403 must not be retried')
        finally:server.shutdown();server.server_close();worker.join()

class Credentials(unittest.TestCase):
    def test_saved_profile_used_without_environment(self):
        with tempfile.TemporaryDirectory() as temporary,patch.dict(os.environ,{'NOTES_API_URL':'','NOTES_API_TOKEN':''}),patch.object(Path,'home',return_value=Path(temporary)):
            from agent_auth import save_profile,load_profile
            token='sj_'+'a'*43
            save_profile({'api_url':'https://notes.example.test/api/v1','token':token})
            self.assertEqual(notes.Client().token,token)
            self.assertEqual(load_profile()['api_url'],'https://notes.example.test/api/v1')
            with self.assertRaises(notes.CliError):notes.Client({'NOTES_API_URL':'https://another.test/api/v1'})
            private=Path(temporary)/'.shiji-notes/client.json'
            if os.name!='nt':
                self.assertEqual(private.stat().st_mode&0o777,0o600)
                private.chmod(0o644)
                with self.assertRaises(ValueError):load_profile()
            else:
                subprocess.run(['icacls',str(private),'/grant','*S-1-1-0:R'],check=True,capture_output=True)
                with self.assertRaises(ValueError):load_profile()
    def test_explicit_environment_remains_supported(self):
        c=notes.Client({'NOTES_API_URL':'https://notes.example.test/api/v1','NOTES_API_TOKEN':'fixture'})
        self.assertEqual(c.token,'fixture')

    def test_public_authorization_requests_do_not_transmit_the_raw_token(self):
        received=[]
        class Handler(BaseHTTPRequestHandler):
            def reply(self):
                self.rfile.read(int(self.headers.get('Content-Length','0')))
                received.append((self.path,self.headers.get('Authorization')))
                self.send_response(200);self.send_header('Content-Type','application/json');self.end_headers();self.wfile.write(b'{}')
            do_POST=reply;do_GET=reply
            def log_message(self,*args):pass
        server=HTTPServer(('127.0.0.1',0),Handler);worker=Thread(target=server.serve_forever,daemon=True);worker.start()
        try:
            token='sj_'+'c'*43
            client=notes.Client({'NOTES_API_URL':f'http://127.0.0.1:{server.server_port}/api/v1','NOTES_API_TOKEN':token,'NOTES_ALLOW_LOCAL_HTTP':'1'})
            for route in ['request','poll','cancel']:client.request('POST','/auth/agent/'+route,{})
            client.request('GET','/auth/agent/me')
            self.assertEqual([header for _,header in received[:3]],[None,None,None])
            self.assertEqual(received[-1][1],'Bearer '+token)
        finally:server.shutdown();server.server_close();worker.join()

    def test_failed_save_or_interruption_preserves_previous_and_cancels_new_grant(self):
        from agent_auth import save_profile,load_profile,login
        with tempfile.TemporaryDirectory() as temporary,patch.dict(os.environ,{'NOTES_API_URL':'','NOTES_API_TOKEN':''}),patch.object(Path,'home',return_value=Path(temporary)):
            old='sj_'+'b'*43;save_profile({'api_url':'https://old.test/api/v1','token':old})
            args=type('Args',(),{'server':'https://new.test','replace':True,'name':'test','no_browser':True})()
            calls=[]
            class Server:
                def __init__(self,env):self.base=env['NOTES_API_URL'];self.token=env['NOTES_API_TOKEN']
                def request(self,method,path,body=None):
                    calls.append(path)
                    if path.endswith('/request'):return {'device_code':'x'*43,'user_code':'ABCD-EFG2','expires_in':600}
                    if path.endswith('/poll'):return {'status':'approved'}
                    if path.endswith('/me'):return {'account':{'id':'test'},'token':{'prefix':self.token[:10],'id':'test','scopes':['notes:read']}}
                    return {'status':'canceled'}
            with patch('agent_auth.save_profile',side_effect=OSError('fixture save failure')),patch('sys.stderr'):
                with self.assertRaises(OSError):login(args,Server,notes.CliError)
            self.assertEqual(load_profile()['token'],old);self.assertIn('/auth/agent/cancel',calls)
            original=Server.request
            def interrupted(self,method,path,body=None):
                if path.endswith('/poll'):raise KeyboardInterrupt()
                return original(self,method,path,body)
            calls.clear()
            with patch.object(Server,'request',interrupted),patch('sys.stderr'):
                with self.assertRaises(KeyboardInterrupt):login(args,Server,notes.CliError)
            self.assertEqual(load_profile()['token'],old);self.assertIn('/auth/agent/cancel',calls)

if __name__=='__main__':unittest.main()
