from __future__ import annotations
import json, subprocess
from pathlib import Path
from .native_export_v6 import build_fixtures

def _parse(stdout:str):
    d=dict(line.split('=',1) for line in stdout.strip().splitlines() if '=' in line)
    return [int(x) for x in d['GREEDY'].split(',') if x],[float(x) for x in d['LOGITS'].split(',') if x]

def run_validation(root='.'):
    root=Path(root).resolve(); fdir=root/'results/native_fixture'; ref=build_fixtures(fdir)
    subprocess.run([str(root/'native/build_host.sh')],check=True,cwd=root)
    cli=root/'native/reasonix-native'; prompt=','.join(map(str,ref['prompt'])); results={}
    for policy,v in ref['variants'].items():
        model=fdir/f'smoke_v06_{policy}.rxm6'; results[policy]={}
        for mode,mref in v['modes'].items():
            cp=subprocess.run([str(cli),str(model),prompt,str(len(mref['greedy'])),mode],check=True,text=True,capture_output=True)
            g,l=_parse(cp.stdout); rl=mref['last_logits']; err=max(abs(a-b) for a,b in zip(l,rl)); mean=sum(abs(a-b) for a,b in zip(l,rl))/len(rl)
            ok=g==mref['greedy'] and err<2e-4
            results[policy][mode]={'greedy_match':g==mref['greedy'],'max_abs_logit_error':err,'mean_abs_logit_error':mean,'pass':ok}
    out={'variants':results,'pass':all(x['pass'] for p in results.values() for x in p.values())}
    (root/'results/native_validation_v06.json').write_text(json.dumps(out,indent=2),encoding='utf-8')
    if not out['pass']: raise AssertionError(out)
    return out
if __name__=='__main__': print(json.dumps(run_validation(),indent=2))
