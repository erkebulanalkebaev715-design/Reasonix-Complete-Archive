#!/usr/bin/env python3
"""Reasonix Android host MCP tools for the Debian/PRoot backend.

This exposes only capabilities actually visible on the phone. It does not fake
or bypass Android grants: Termux:API permissions, Shizuku/rish and root/su still
control what succeeds. MCP annotations let Reasonix keep mutating operations
behind its permission layer.
"""
from __future__ import annotations
import argparse, json, os, pathlib, shutil, subprocess, sys

PROTO="2024-11-05"
TERMUX_PREFIX=pathlib.Path('/data/data/com.termux/files/usr/bin')
TERMUX_NAMES=[
 'termux-battery-status','termux-clipboard-get','termux-clipboard-set','termux-notification',
 'termux-toast','termux-vibrate','termux-open-url','termux-wifi-connectioninfo','termux-camera-photo',
 'termux-location','termux-sensor','termux-volume','termux-brightness','termux-torch','termux-media-player',
 'termux-share','termux-open','termux-telephony-deviceinfo'
]

def which(name):
    p=shutil.which(name)
    if p:return p
    q=TERMUX_PREFIX/name
    return str(q) if q.exists() and os.access(q,os.X_OK) else ''

def shared_roots():
    out=[]
    for raw in ['/sdcard','/storage/emulated/0','/data/data/com.termux/files/home/storage/shared']:
        p=pathlib.Path(raw)
        try:
            if p.exists():
                rp=p.resolve()
                if rp not in out:out.append(rp)
        except Exception:pass
    return out

def caps():
    termux={n:which(n) for n in TERMUX_NAMES};rish=which('rish');su=which('su');system_sh='/system/bin/sh' if pathlib.Path('/system/bin/sh').exists() else ''
    return {'termuxApi':bool(any(termux.values())),'termuxTools':sorted(k for k,v in termux.items() if v),
      'shizukuRish':bool(rish),'rishPath':rish,'rootSu':bool(su),'suPath':su,
      'androidSystemShell':bool(system_sh),'systemShellPath':system_sh,
      'sharedStorage':[str(x) for x in shared_roots()],
      'note':'Capabilities are runtime facts from Debian/PRoot; Android grants/root/Shizuku still decide whether each call succeeds.'}

def run(cmd,timeout=20,input_text=None,shell=False):
    try:
        cp=subprocess.run(cmd,shell=shell,input=input_text,text=True,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=max(1,min(int(timeout),60)))
        return {'exitCode':cp.returncode,'stdout':cp.stdout[-200000:],'stderr':cp.stderr[-50000:]}
    except subprocess.TimeoutExpired as e:return {'exitCode':124,'stdout':(e.stdout or '')[-200000:] if isinstance(e.stdout,str) else '','stderr':'timeout'}
    except Exception as e:return {'exitCode':127,'stdout':'','stderr':str(e)}

def termux_call(name,args=None,input_text=None,timeout=20):
    p=which(name)
    if not p:return {'exitCode':127,'stdout':'','stderr':f'{name} unavailable'}
    return run([p]+(args or []),timeout,input_text)

def ro(name,desc,props=None,required=None):
    return {'name':name,'description':desc,'inputSchema':{'type':'object','properties':props or {},**({'required':required} if required else {})},'annotations':{'readOnlyHint':True}}
def rw(name,desc,props=None,required=None,destructive=False):
    return {'name':name,'description':desc,'inputSchema':{'type':'object','properties':props or {},**({'required':required} if required else {})},'annotations':{'readOnlyHint':False,'destructiveHint':bool(destructive)}}

def tool_defs():
    S={'type':'string'}; I={'type':'integer'}
    return [
      ro('capabilities','Inspect real Android host capabilities visible from Reasonix Debian/PRoot.'),
      rw('host_shell','Run a user-approved command through Android shell, Shizuku/rish or root/su when available.',{'command':S,'mode':{'type':'string','enum':['auto','android','shizuku','root']},'timeout':{'type':'integer','minimum':1,'maximum':60}},['command'],True),
      ro('battery_status','Read Android battery status through Termux:API.'),ro('clipboard_get','Read Android clipboard through Termux:API.'),
      rw('clipboard_set','Set Android clipboard through Termux:API.',{'text':S},['text']),rw('notify','Post an Android notification.',{'title':S,'content':S},['content']),
      rw('toast','Show an Android toast.',{'text':S},['text']),rw('vibrate','Vibrate the phone.',{'milliseconds':{'type':'integer','minimum':10,'maximum':5000}}),
      ro('wifi_info','Read Wi-Fi connection info.'),ro('location','Read current device location using Termux:API.',{'provider':{'type':'string','enum':['gps','network','passive']}}),
      ro('device_info','Read telephony/device information exposed by Termux:API.'),ro('sensor_list','List Android sensors exposed by Termux:API.'),
      ro('sensor_read','Read one Android sensor.',{'sensor':S,'limit':{'type':'integer','minimum':1,'maximum':20}},['sensor']),
      rw('camera_photo','Capture a photo using Termux:API into shared storage.',{'path':S,'camera':{'type':'integer','minimum':0,'maximum':8}},None),
      rw('torch','Turn flashlight on/off through Termux:API.',{'enabled':{'type':'boolean'}},['enabled']),
      rw('brightness','Set display brightness (0-255 or auto) through Termux:API.',{'value':{'oneOf':[I,{'type':'string','enum':['auto']}] }},['value']),
      ro('volume_get','Read Android audio stream volumes.'),rw('volume_set','Set Android stream volume.',{'stream':S,'volume':I},['stream','volume']),
      rw('open_url','Open an http(s) URL on Android.',{'url':S},['url']),rw('open_file','Open a shared-storage file with an Android app.',{'path':S},['path']),
      rw('share','Open Android share sheet for text or a shared file.',{'text':S,'path':S}),
      rw('media_play','Control Termux media player.',{'action':{'type':'string','enum':['play','pause','stop','info']},'path':S},['action']),
      ro('shared_list','List files inside Android shared storage.',{'path':S,'limit':{'type':'integer','minimum':1,'maximum':500}}),
      ro('shared_read_text','Read a UTF-8 text file from Android shared storage.',{'path':S,'maxBytes':{'type':'integer','minimum':1,'maximum':524288}},['path']),
      rw('shared_write_text','Write a UTF-8 text file inside Android shared storage.',{'path':S,'text':S,'append':{'type':'boolean'}},['path','text'],True),
    ]

def safe_shared(raw,for_write=False):
    roots=shared_roots()
    if not roots:raise ValueError('shared storage is not visible from Debian/PRoot')
    p=pathlib.Path(str(raw or '').strip())
    if not p.is_absolute():p=roots[0]/p
    # Resolve parent when creating a new file, otherwise resolve the path itself.
    try:rp=(p.parent.resolve()/p.name) if for_write and not p.exists() else p.resolve()
    except Exception:rp=p.absolute()
    for r in roots:
        try:rp.relative_to(r);return rp
        except ValueError:pass
    raise ValueError('path is outside shared storage')

def call(name,a):
    a=a or {}
    if name=='capabilities':return caps(),False
    if name=='host_shell':
        command=str(a.get('command','')).strip();mode=str(a.get('mode','auto'));timeout=int(a.get('timeout',20) or 20)
        if not command:return {'error':'command required'},True
        c=caps();runner=None
        if mode in ('auto','root') and c['rootSu']:runner=[c['suPath'],'-c',command]
        if runner is None and mode in ('auto','shizuku') and c['shizukuRish']:runner=[c['rishPath'],'-c',command]
        if runner is None and mode in ('auto','android') and c['androidSystemShell']:runner=[c['systemShellPath'],'-c',command]
        if runner is None:return {'error':f'no host shell available for mode={mode}','capabilities':c},True
        out=run(runner,timeout);return out,out.get('exitCode')!=0
    if name=='battery_status':r=termux_call('termux-battery-status');return r,r['exitCode']!=0
    if name=='clipboard_get':r=termux_call('termux-clipboard-get');return r,r['exitCode']!=0
    if name=='clipboard_set':r=termux_call('termux-clipboard-set',input_text=str(a.get('text','')));return r,r['exitCode']!=0
    if name=='notify':
        args=[];title=str(a.get('title','')).strip();content=str(a.get('content',''))
        if title:args+=['--title',title]
        args+=['--content',content];r=termux_call('termux-notification',args);return r,r['exitCode']!=0
    if name=='toast':r=termux_call('termux-toast',[str(a.get('text',''))]);return r,r['exitCode']!=0
    if name=='vibrate':r=termux_call('termux-vibrate',['-d',str(max(10,min(int(a.get('milliseconds',250) or 250),5000)))]);return r,r['exitCode']!=0
    if name=='wifi_info':r=termux_call('termux-wifi-connectioninfo');return r,r['exitCode']!=0
    if name=='location':
        args=['-p',str(a.get('provider','gps'))];r=termux_call('termux-location',args,timeout=30);return r,r['exitCode']!=0
    if name=='device_info':r=termux_call('termux-telephony-deviceinfo');return r,r['exitCode']!=0
    if name=='sensor_list':r=termux_call('termux-sensor',['-l']);return r,r['exitCode']!=0
    if name=='sensor_read':r=termux_call('termux-sensor',['-s',str(a.get('sensor','')),'-n',str(max(1,min(int(a.get('limit',1) or 1),20)))],timeout=30);return r,r['exitCode']!=0
    if name=='camera_photo':
        dest=safe_shared(a.get('path') or 'DCIM/Reasonix/reasonix-photo.jpg',True);dest.parent.mkdir(parents=True,exist_ok=True)
        r=termux_call('termux-camera-photo',['-c',str(int(a.get('camera',0) or 0)),str(dest)],timeout=40);r['path']=str(dest);return r,r['exitCode']!=0
    if name=='torch':r=termux_call('termux-torch',['on' if bool(a.get('enabled')) else 'off']);return r,r['exitCode']!=0
    if name=='brightness':r=termux_call('termux-brightness',[str(a.get('value'))]);return r,r['exitCode']!=0
    if name=='volume_get':r=termux_call('termux-volume');return r,r['exitCode']!=0
    if name=='volume_set':r=termux_call('termux-volume',[str(a.get('stream','music')),str(int(a.get('volume',0)))]);return r,r['exitCode']!=0
    if name=='open_url':
        url=str(a.get('url','')).strip()
        if not (url.startswith('https://') or url.startswith('http://')):return {'error':'only http(s) URLs accepted'},True
        r=termux_call('termux-open-url',[url]);return r,r['exitCode']!=0
    if name=='open_file':
        p=safe_shared(a.get('path'));r=termux_call('termux-open',[str(p)]);return r,r['exitCode']!=0
    if name=='share':
        path=str(a.get('path','')).strip();text=str(a.get('text',''))
        if path:
            p=safe_shared(path);r=termux_call('termux-share',[str(p)])
        else:r=termux_call('termux-share',input_text=text)
        return r,r['exitCode']!=0
    if name=='media_play':
        action=str(a.get('action','info'));args=[action]
        if action=='play':
            p=safe_shared(a.get('path'));args=[action,str(p)]
        r=termux_call('termux-media-player',args);return r,r['exitCode']!=0
    if name=='shared_list':
        p=safe_shared(a.get('path') or '.');limit=max(1,min(int(a.get('limit',200) or 200),500));items=[]
        try:
            for x in sorted(p.iterdir(),key=lambda q:(not q.is_dir(),q.name.lower()))[:limit]:items.append({'name':x.name,'path':str(x),'dir':x.is_dir(),'size':x.stat().st_size if x.is_file() else None})
            return {'path':str(p),'items':items},False
        except Exception as e:return {'error':str(e)},True
    if name=='shared_read_text':
        p=safe_shared(a.get('path'));mx=max(1,min(int(a.get('maxBytes',262144) or 262144),524288))
        try:
            b=p.read_bytes();truncated=len(b)>mx;b=b[:mx];return {'path':str(p),'text':b.decode('utf-8','replace'),'truncated':truncated,'bytes':len(b)},False
        except Exception as e:return {'error':str(e)},True
    if name=='shared_write_text':
        p=safe_shared(a.get('path'),True);text=str(a.get('text',''))
        if len(text.encode('utf-8'))>1024*1024:return {'error':'text exceeds 1 MiB'},True
        try:
            p.parent.mkdir(parents=True,exist_ok=True);mode='a' if bool(a.get('append')) else 'w'
            with p.open(mode,encoding='utf-8') as f:f.write(text)
            return {'path':str(p),'bytes':len(text.encode('utf-8')),'append':bool(a.get('append'))},False
        except Exception as e:return {'error':str(e)},True
    return {'error':'unknown tool'},True

def text_result(obj,err=False):return {'content':[{'type':'text','text':json.dumps(obj,ensure_ascii=False,indent=2)}],'isError':bool(err)}
def handle(req):
    if 'id' not in req:return None
    rid=req.get('id');m=req.get('method');p=req.get('params') or {}
    if m=='initialize':result={'protocolVersion':PROTO,'capabilities':{'tools':{}},'serverInfo':{'name':'reasonix-android-host','version':'1.2'}}
    elif m=='tools/list':result={'tools':tool_defs()}
    elif m=='tools/call':obj,err=call(str(p.get('name','')),p.get('arguments') or {});result=text_result(obj,err)
    else:return {'jsonrpc':'2.0','id':rid,'error':{'code':-32601,'message':'method not found: '+str(m)}}
    return {'jsonrpc':'2.0','id':rid,'result':result}
def serve():
    for line in sys.stdin:
        line=line.strip()
        if not line:continue
        try:req=json.loads(line);resp=handle(req)
        except Exception as e:resp={'jsonrpc':'2.0','id':None,'error':{'code':-32603,'message':str(e)}}
        if resp is not None:sys.stdout.write(json.dumps(resp,separators=(',',':'),ensure_ascii=False)+'\n');sys.stdout.flush()
def main():
    ap=argparse.ArgumentParser();ap.add_argument('--capabilities-json',action='store_true');a=ap.parse_args()
    if a.capabilities_json:print(json.dumps(caps(),ensure_ascii=False));return
    serve()
if __name__=='__main__':main()
