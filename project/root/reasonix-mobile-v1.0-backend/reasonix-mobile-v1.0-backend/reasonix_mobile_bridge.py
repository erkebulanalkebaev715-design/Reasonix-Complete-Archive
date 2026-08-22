#!/usr/bin/env python3
"""Reasonix Mobile v1 loopback bridge/supervisor.

No agent logic lives here. This process only:
- owns/restarts the existing `reasonix serve` child,
- authenticates to it with the supervisor token,
- exposes a CORS-safe loopback facade for the APK WebView,
- lists/switches configured Reasonix models,
- writes user-created native Reasonix project skills under .reasonix/skills.
"""
from __future__ import annotations
import argparse, atexit, hmac, http.cookiejar, json, os, re, signal, subprocess, sys, threading, time
import urllib.error, urllib.request
import tomllib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlsplit

MAX_BODY = 4 * 1024 * 1024
SKILL_MAX = 128 * 1024
SAFE_SKILL = re.compile(r'^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$')
SAFE_MCP = re.compile(r'^[A-Za-z_][A-Za-z0-9_-]{0,63}$')

class MobileCore:
    def __init__(self, root: str, binary: str, state: str, token_file: str, model: str):
        self.root = Path(root).resolve(); self.binary = str(Path(binary).resolve()); self.state = Path(state).resolve()
        self.token_file = Path(token_file).resolve(); self.token = self.token_file.read_text(encoding='utf-8').strip()
        self.model_file = self.state / 'model'; self.up_port = self.state / 'reasonix.port'; self.up_pid = self.state / 'reasonix.pid'; self.up_log = self.state / 'reasonix.log'
        self.lock = threading.RLock(); self.proc: subprocess.Popen | None = None; self.upstream = ''; self.jar = None; self.opener = None
        persisted = self.model_file.read_text(encoding='utf-8').strip() if self.model_file.exists() else ''
        self.model = persisted or model
        self.env = os.environ.copy(); self.env['PATH'] = '/usr/local/go/bin:' + self.env.get('PATH','/usr/bin:/bin'); self.env['GOTOOLCHAIN'] = 'local'
        self.start_upstream(self.model)
        atexit.register(self.stop_upstream)

    def _read(self,p):
        try:return Path(p).read_text(encoding='utf-8').strip()
        except Exception:return ''

    def start_upstream(self, model: str):
        with self.lock:
            self.stop_upstream()
            for p in (self.up_port,self.up_pid):
                try:p.unlink()
                except FileNotFoundError:pass
            log = open(self.up_log,'ab', buffering=0)
            args=[self.binary,'serve','--model',model,'--addr','127.0.0.1:0','--auth','token','--port-file',str(self.up_port),'--token-file',str(self.token_file),'--pid-file',str(self.up_pid)]
            self.proc=subprocess.Popen(args,cwd=self.root,env=self.env,stdout=log,stderr=subprocess.STDOUT,start_new_session=True)
            deadline=time.time()+25
            while time.time()<deadline:
                if self.proc.poll() is not None: raise RuntimeError(f'reasonix serve exited rc={self.proc.returncode}; see {self.up_log}')
                if self.up_port.exists() and self.up_pid.exists() and self._read(self.up_port): break
                time.sleep(.1)
            addr=self._read(self.up_port)
            if not addr.startswith('127.0.0.1:'):
                self.stop_upstream(); raise RuntimeError(f'bad/non-loopback reasonix address: {addr!r}')
            self.upstream='http://'+addr
            self.jar=http.cookiejar.CookieJar(); self.opener=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(self.jar))
            self._bootstrap_auth()
            # Prove the selected model actually produced a live backend before committing it.
            code,_,_=self._request_upstream('GET','/mod/status',b'',None,timeout=8)
            if code!=200:
                self.stop_upstream(); raise RuntimeError(f'reasonix status HTTP {code}')
            self.model=model; self.model_file.write_text(model+'\n',encoding='utf-8')

    def stop_upstream(self):
        with self.lock:
            p=self.proc
            if p and p.poll() is None:
                try:p.terminate(); p.wait(timeout=5)
                except Exception:
                    try:p.kill()
                    except Exception:pass
            self.proc=None
            pid=self._read(self.up_pid)
            if pid.isdigit():
                try:os.kill(int(pid),signal.SIGTERM)
                except Exception:pass
            for f in (self.up_port,self.up_pid):
                try:f.unlink()
                except FileNotFoundError:pass

    def _bootstrap_auth(self):
        data=json.dumps({'token':self.token}).encode()
        req=urllib.request.Request(self.upstream+'/auth/token',data=data,headers={'Content-Type':'application/json'},method='POST')
        with self.opener.open(req,timeout=8) as r:
            if r.status!=204: raise RuntimeError(f'upstream auth HTTP {r.status}')

    def _request_upstream(self,method,path,body,ctype,timeout=65):
        headers={'Accept':'application/json, text/plain, */*'}
        if ctype:headers['Content-Type']=ctype
        req=urllib.request.Request(self.upstream+path,data=body if method!='GET' else None,headers=headers,method=method)
        try:
            with self.opener.open(req,timeout=timeout) as r:
                out=r.read(MAX_BODY+1)
                if len(out)>MAX_BODY:return 502,{'Content-Type':'text/plain'},b'Upstream response too large\n'
                return r.status,{'Content-Type':r.headers.get('Content-Type','application/octet-stream'),'Cache-Control':'no-store'},out
        except urllib.error.HTTPError as e:
            out=e.read(MAX_BODY+1)
            return e.code,{'Content-Type':e.headers.get('Content-Type','text/plain; charset=utf-8'),'Cache-Control':'no-store'},out[:MAX_BODY]

    def forward(self,method,path,body,ctype):
        if not path.startswith('/mod/'):return 404,{'Content-Type':'text/plain'},b'Not Found\n'
        with self.lock:
            if not self.proc or self.proc.poll() is not None:return 503,{'Content-Type':'text/plain'},b'Reasonix backend stopped\n'
            return self._request_upstream(method,path,body,ctype)

    def doctor(self):
        cp=subprocess.run([self.binary,'doctor','--json'],cwd=self.root,env=self.env,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True,timeout=25)
        if cp.returncode!=0:raise RuntimeError((cp.stderr or cp.stdout or 'doctor failed')[-2000:])
        return json.loads(cp.stdout)

    def models(self):
        d=self.doctor(); out=[]
        for p in d.get('providers',[]) or []:
            name=str(p.get('name','')).strip(); kind=p.get('kind',''); ready=bool(p.get('key_present',False) or kind=='mock')
            ms=p.get('models') or ([p.get('model')] if p.get('model') else [])
            for m in ms:
                if name and m: out.append({'ref':f'{name}/{m}','provider':name,'model':m,'kind':kind,'ready':ready,'default':bool(p.get('is_default',False))})
        return {'current':self.model,'models':out}

    def switch_model(self,ref):
        with self.lock:
            avail={x['ref']:x for x in self.models()['models']}
            if ref not in avail:raise ValueError('model ref not configured in Reasonix')
            if not avail[ref]['ready']:raise ValueError('provider is not ready')
            old=self.model
            if ref==old:return {'ok':True,'model':ref,'changed':False}
            try:self.start_upstream(ref)
            except Exception as e:
                try:self.start_upstream(old)
                except Exception:pass
                raise RuntimeError(f'model switch failed; rollback attempted: {e}')
            return {'ok':True,'model':ref,'changed':True}

    def add_provider(self,obj):
        name=str(obj.get('name','')).strip();kind=str(obj.get('kind','openai')).strip().lower();base=str(obj.get('baseUrl','')).strip().rstrip('/')
        models=obj.get('models') or []; env_name=str(obj.get('apiKeyEnv','')).strip(); key=str(obj.get('apiKey',''))
        try:ctx=int(obj.get('contextWindow') or 128000)
        except Exception:raise ValueError('bad contextWindow')
        if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_-]{0,47}',name):raise ValueError('provider name: letters/numbers/_/-, max 48')
        if kind not in ('openai','anthropic'):raise ValueError('kind must be openai|anthropic')
        if not (base.startswith('https://') or base.startswith('http://127.0.0.1') or base.startswith('http://localhost')):raise ValueError('baseUrl must be https:// or local loopback http://')
        if isinstance(models,str):models=[x.strip() for x in models.split(',') if x.strip()]
        if not isinstance(models,list) or not models:raise ValueError('at least one model required')
        models=[str(x).strip() for x in models if str(x).strip()]
        if not models or len(models)>32 or any(len(x)>160 or any(c in x for c in '\r\n"') for x in models):raise ValueError('invalid model list')
        if not env_name:env_name=re.sub(r'[^A-Za-z0-9_]','_',name.upper())+'_API_KEY'
        if not re.fullmatch(r'[A-Za-z_][A-Za-z0-9_]{0,127}',env_name):raise ValueError('invalid API key variable name')
        if ctx<4096 or ctx>4000000:raise ValueError('contextWindow out of range')
        cfg=self.root/'reasonix.toml'; old_cfg=cfg.read_bytes() if cfg.exists() else b''
        try: parsed=tomllib.loads(old_cfg.decode('utf-8')) if old_cfg else {}
        except Exception as e:raise ValueError(f'existing reasonix.toml is invalid: {e}')
        if any(str(x.get('name',''))==name for x in (parsed.get('providers') or [])):raise ValueError('provider already exists')
        import json as _json
        block=['','[[providers]]',f'name = {_json.dumps(name)}',f'kind = {_json.dumps(kind)}',f'base_url = {_json.dumps(base)}',
               'models = ['+', '.join(_json.dumps(x) for x in models)+']',f'default = {_json.dumps(models[0])}',f'api_key_env = {_json.dumps(env_name)}',f'context_window = {ctx}','']
        cfg.write_text(old_cfg.decode('utf-8')+'\n'.join(block),encoding='utf-8')
        env_path=Path(os.environ.get('REASONIX_HOME') or (Path.home()/'.reasonix'))/'.env'; env_path.parent.mkdir(parents=True,exist_ok=True)
        old_env=env_path.read_bytes() if env_path.exists() else None
        try:
            if key:
                if '\n' in key or '\r' in key:raise ValueError('API key contains newline')
                lines=env_path.read_text(encoding='utf-8').splitlines() if env_path.exists() else []; out=[];done=False
                for line in lines:
                    if line.startswith(env_name+'='):out.append(env_name+'='+key);done=True
                    else:out.append(line)
                if not done:out.append(env_name+'='+key)
                env_path.write_text('\n'.join(out)+'\n',encoding='utf-8');os.chmod(env_path,0o600)
            self.doctor()
        except Exception:
            cfg.write_bytes(old_cfg)
            if old_env is None:
                try:env_path.unlink()
                except FileNotFoundError:pass
            else:env_path.write_bytes(old_env)
            raise
        return {'ok':True,'provider':name,'models':[f'{name}/{m}' for m in models],'apiKeyEnv':env_name,'keyStored':bool(key)}

    @property
    def skills_root(self):return self.root/'.reasonix'/'skills'
    def list_skills(self):
        root=self.skills_root; items=[]
        if root.exists():
            for f in sorted(root.glob('*/SKILL.md')):
                txt=f.read_text(encoding='utf-8',errors='replace');desc=''
                m=re.search(r'(?m)^description:\s*["\']?(.*?)["\']?\s*$',txt)
                if m:desc=m.group(1)
                items.append({'name':f.parent.name,'description':desc,'path':str(f.relative_to(self.root))})
            for f in sorted(root.glob('*.md')):
                items.append({'name':f.stem,'description':'','path':str(f.relative_to(self.root))})
        return {'skills':items}
    def save_skill(self,obj):
        name=str(obj.get('name','')).strip(); body=str(obj.get('body','')).strip(); desc=str(obj.get('description','')).strip()[:240]
        run_as=str(obj.get('runAs','inline')).strip(); invocation=str(obj.get('invocation','auto')).strip()
        if not SAFE_SKILL.fullmatch(name):raise ValueError('skill name: letters/numbers/_/-, max 64')
        if not body:raise ValueError('skill body is empty')
        if len(body.encode())>SKILL_MAX:raise ValueError('skill body too large')
        if run_as not in ('inline','subagent'):raise ValueError('runAs must be inline|subagent')
        if invocation not in ('auto','manual'):raise ValueError('invocation must be auto|manual')
        if not desc:desc=f'User skill {name}'
        allowed=obj.get('allowedTools') or []
        if not isinstance(allowed,list):allowed=[]
        allowed=[str(x).strip() for x in allowed if str(x).strip()][:64]
        fm=['---',f'name: {name}',f'description: {json.dumps(desc,ensure_ascii=False)}',f'runAs: {run_as}',f'invocation: {invocation}']
        if allowed:fm.append('allowed-tools: ['+', '.join(json.dumps(x) for x in allowed)+']')
        fm+=['---','',body,'']
        dest=self.skills_root/name/'SKILL.md';dest.parent.mkdir(parents=True,exist_ok=True)
        tmp=dest.with_suffix('.tmp');tmp.write_text('\n'.join(fm),encoding='utf-8');os.replace(tmp,dest)
        return {'ok':True,'name':name,'path':str(dest.relative_to(self.root))}
    def delete_skill(self,name):
        if not SAFE_SKILL.fullmatch(name):raise ValueError('bad skill name')
        dest=self.skills_root/name/'SKILL.md'
        if dest.exists():dest.unlink()
        try:dest.parent.rmdir()
        except OSError:pass
        return {'ok':True,'name':name}

    @property
    def mcp_path(self):return self.root/'.mcp.json'
    def _load_mcp_doc(self):
        if not self.mcp_path.exists():return {'mcpServers':{}}
        try:doc=json.loads(self.mcp_path.read_text(encoding='utf-8'))
        except Exception as e:raise ValueError(f'.mcp.json is invalid: {e}')
        if not isinstance(doc,dict):raise ValueError('.mcp.json root must be an object')
        servers=doc.get('mcpServers')
        if servers is None:doc['mcpServers']={}
        elif not isinstance(servers,dict):raise ValueError('.mcp.json mcpServers must be an object')
        return doc
    def list_mcp_tools(self):
        doc=self._load_mcp_doc();out=[]
        for name,spec in sorted((doc.get('mcpServers') or {}).items()):
            if not isinstance(spec,dict):continue
            transport='stdio' if spec.get('command') else str(spec.get('type') or 'remote')
            out.append({'name':name,'transport':transport,'command':str(spec.get('command') or ''),'args':spec.get('args') or [],
                        'url':str(spec.get('url') or ''),'envKeys':sorted((spec.get('env') or {}).keys()) if isinstance(spec.get('env'),dict) else [],
                        'headerKeys':sorted((spec.get('headers') or {}).keys()) if isinstance(spec.get('headers'),dict) else [],
                        'autoStart':spec.get('auto_start')})
        return {'servers':out,'path':str(self.mcp_path.relative_to(self.root))}
    @staticmethod
    def _clean_map(v,label):
        if v in (None,''):return {}
        if not isinstance(v,dict):raise ValueError(f'{label} must be JSON object')
        if len(v)>64:raise ValueError(f'{label} has too many entries')
        out={}
        for k,val in v.items():
            k=str(k).strip(); val=str(val)
            if not k or len(k)>128 or any(c in k for c in '\r\n\0') or len(val)>8192 or any(c in val for c in '\r\n\0'):
                raise ValueError(f'invalid {label} entry')
            out[k]=val
        return out
    def save_mcp_tool(self,obj):
        name=str(obj.get('name','')).strip();transport=str(obj.get('transport','stdio')).strip().lower()
        if not SAFE_MCP.fullmatch(name):raise ValueError('tool server name: letters/numbers/_/-, max 64')
        if transport not in ('stdio','sse','http'):raise ValueError('transport must be stdio|sse|http')
        doc=self._load_mcp_doc();servers=doc.setdefault('mcpServers',{})
        if name in servers and not bool(obj.get('replace',False)):raise ValueError('tool server already exists')
        auto=bool(obj.get('autoStart',True));spec={'auto_start':auto}
        if transport=='stdio':
            cmd=str(obj.get('command','')).strip();args=obj.get('args') or []
            if not cmd or len(cmd)>1024 or any(c in cmd for c in '\r\n\0'):raise ValueError('stdio command required')
            if isinstance(args,str):args=[x for x in args.split(' ') if x]
            if not isinstance(args,list) or len(args)>128:raise ValueError('args must be an array')
            args=[str(x) for x in args]
            if any(len(x)>4096 or any(c in x for c in '\r\n\0') for x in args):raise ValueError('invalid arg')
            spec.update({'command':cmd,'args':args})
            env=self._clean_map(obj.get('env'),'env')
            if env:spec['env']=env
        else:
            url=str(obj.get('url','')).strip()
            if not (url.startswith('https://') or url.startswith('http://127.0.0.1') or url.startswith('http://localhost')):raise ValueError('remote MCP URL must be https:// or local loopback http://')
            if len(url)>4096 or any(c in url for c in '\r\n\0'):raise ValueError('invalid MCP URL')
            spec.update({'type':transport,'url':url})
            headers=self._clean_map(obj.get('headers'),'headers')
            if headers:spec['headers']=headers
        old=self.mcp_path.read_bytes() if self.mcp_path.exists() else None
        servers[name]=spec
        tmp=self.mcp_path.with_suffix('.json.tmp');tmp.write_text(json.dumps(doc,ensure_ascii=False,indent=2)+'\n',encoding='utf-8');os.replace(tmp,self.mcp_path)
        try:
            self.doctor()
            self.start_upstream(self.model)
        except Exception:
            if old is None:
                try:self.mcp_path.unlink()
                except FileNotFoundError:pass
            else:self.mcp_path.write_bytes(old)
            try:self.start_upstream(self.model)
            except Exception:pass
            raise
        return {'ok':True,'name':name,'transport':transport,'reloaded':True}
    def delete_mcp_tool(self,name):
        if not SAFE_MCP.fullmatch(name):raise ValueError('bad tool server name')
        doc=self._load_mcp_doc();servers=doc.get('mcpServers') or {}
        if name not in servers:return {'ok':True,'name':name,'deleted':False}
        old=self.mcp_path.read_bytes() if self.mcp_path.exists() else None
        del servers[name];doc['mcpServers']=servers
        tmp=self.mcp_path.with_suffix('.json.tmp');tmp.write_text(json.dumps(doc,ensure_ascii=False,indent=2)+'\n',encoding='utf-8');os.replace(tmp,self.mcp_path)
        try:self.doctor();self.start_upstream(self.model)
        except Exception:
            if old is not None:self.mcp_path.write_bytes(old)
            try:self.start_upstream(self.model)
            except Exception:pass
            raise
        return {'ok':True,'name':name,'deleted':True,'reloaded':True}


def make_handler(core:MobileCore):
    class H(BaseHTTPRequestHandler):
        server_version='ReasonixMobileBridge/1.0'
        def log_message(self,fmt,*args):sys.stderr.write('[bridge] '+fmt%args+'\n')
        def cors(self):
            self.send_header('Access-Control-Allow-Origin','*');self.send_header('Access-Control-Allow-Methods','GET, POST, OPTIONS')
            self.send_header('Access-Control-Allow-Headers','Content-Type, X-Reasonix-Mobile-Token');self.send_header('Access-Control-Max-Age','600')
        def token_ok(self):
            got=self.headers.get('X-Reasonix-Mobile-Token','');return bool(got) and hmac.compare_digest(got,core.token)
        def send_bytes(self,status,headers,body):
            self.send_response(status);self.cors()
            for k,v in headers.items():self.send_header(k,v)
            self.send_header('Content-Length',str(len(body)));self.end_headers()
            if self.command!='HEAD':self.wfile.write(body)
        def send_json(self,status,obj):self.send_bytes(status,{'Content-Type':'application/json','Cache-Control':'no-store'},json.dumps(obj,ensure_ascii=False).encode())
        def read_json(self,limit=MAX_BODY):
            n=int(self.headers.get('Content-Length','0') or 0)
            if n>limit:raise ValueError('request too large')
            return json.loads((self.rfile.read(n) if n else b'{}').decode())
        def do_OPTIONS(self):self.send_response(204);self.cors();self.send_header('Content-Length','0');self.end_headers()
        def do_GET(self):self.handle_api('GET')
        def do_POST(self):self.handle_api('POST')
        def handle_api(self,method):
            u=urlsplit(self.path);path=u.path+(('?'+u.query) if u.query else '')
            if u.path=='/mobile/health':self.send_json(200,{'ok':True,'bridge':'reasonix-mobile-v1','model':core.model});return
            if not self.token_ok():self.send_bytes(401,{'Content-Type':'text/plain; charset=utf-8','Cache-Control':'no-store'},b'Unauthorized\n');return
            try:
                if u.path=='/auth/token' and method=='POST':
                    obj=self.read_json(8192);self.send_bytes(204 if hmac.compare_digest(str(obj.get('token','')),core.token) else 401,{'Cache-Control':'no-store'},b'');return
                if u.path=='/mobile/models' and method=='GET':self.send_json(200,core.models());return
                if u.path=='/mobile/model' and method=='POST':
                    obj=self.read_json();self.send_json(200,core.switch_model(str(obj.get('model','')).strip()));return
                if u.path=='/mobile/provider' and method=='POST':self.send_json(200,core.add_provider(self.read_json()));return
                if u.path=='/mobile/tools' and method=='GET':self.send_json(200,core.list_mcp_tools());return
                if u.path=='/mobile/tools' and method=='POST':self.send_json(200,core.save_mcp_tool(self.read_json()));return
                if u.path=='/mobile/tools/delete' and method=='POST':self.send_json(200,core.delete_mcp_tool(str(self.read_json().get('name','')).strip()));return
                if u.path=='/mobile/skills' and method=='GET':self.send_json(200,core.list_skills());return
                if u.path=='/mobile/skills' and method=='POST':self.send_json(200,core.save_skill(self.read_json(SKILL_MAX+8192)));return
                if u.path=='/mobile/skills/delete' and method=='POST':self.send_json(200,core.delete_skill(str(self.read_json().get('name','')).strip()));return
                if u.path=='/mobile/info' and method=='GET':
                    self.send_json(200,{'model':core.model,'upstream':core.upstream,'root':str(core.root),'skillsRoot':str(core.skills_root)});return
                n=int(self.headers.get('Content-Length','0') or 0)
                if n>MAX_BODY:self.send_bytes(413,{'Content-Type':'text/plain'},b'Too Large\n');return
                body=self.rfile.read(n) if n else b''
                status,headers,out=core.forward(method,path,body,self.headers.get('Content-Type'));self.send_bytes(status,headers,out)
            except ValueError as e:self.send_json(400,{'error':str(e)})
            except Exception as e:self.send_json(500,{'error':str(e)})
    return H


def main():
    ap=argparse.ArgumentParser();ap.add_argument('--root',required=True);ap.add_argument('--binary',required=True);ap.add_argument('--state',required=True)
    ap.add_argument('--token-file',required=True);ap.add_argument('--model',required=True);ap.add_argument('--addr',default='127.0.0.1:37914');ap.add_argument('--port-file');ap.add_argument('--pid-file')
    a=ap.parse_args();core=MobileCore(a.root,a.binary,a.state,a.token_file,a.model);host,port=a.addr.rsplit(':',1);srv=ThreadingHTTPServer((host,int(port)),make_handler(core));actual=f'{srv.server_address[0]}:{srv.server_address[1]}'
    if a.port_file:Path(a.port_file).write_text(actual+'\n',encoding='utf-8')
    if a.pid_file:Path(a.pid_file).write_text(str(os.getpid())+'\n',encoding='utf-8')
    print(f'REASONIX_MOBILE_BRIDGE_READY {actual} model={core.model}',flush=True)
    signal.signal(signal.SIGTERM,lambda *_:sys.exit(0));signal.signal(signal.SIGINT,lambda *_:sys.exit(0))
    try:srv.serve_forever()
    finally:core.stop_upstream();srv.server_close()

if __name__=='__main__':main()
