#!/usr/bin/env bash
set -euo pipefail
TARGET="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PAYLOAD="$HERE/payload"
PY_BIN="${PY_BIN:-python3}"

[[ -d "$TARGET/.git" ]] || { echo "ERROR: Reasonix git tree not found: $TARGET" >&2; exit 2; }
cd "$TARGET"
[[ -f internal/serve/mod_bridge.go && -f internal/agent/run_budget.go && -f internal/agent/sampling_request.go && -f internal/provider/retry.go && -f internal/provider/openai/openai.go ]] || {
  echo 'ERROR: required v0.20 core files missing' >&2; exit 3;
}
grep -q 'const balanceModVersion = "balance-mod-v0.20"' internal/serve/mod_bridge.go || {
  echo 'ERROR: Fixed6 requires installed balance-mod-v0.20' >&2; exit 3;
}

STOPPED="$($PY_BIN - <<'PYINNER'
import os,signal
me=os.getpid(); parent=os.getppid(); stopped=0
for name in os.listdir('/proc'):
    if not name.isdigit(): continue
    pid=int(name)
    if pid in (me,parent): continue
    try: argv=[x.decode('utf-8','replace') for x in open(f'/proc/{pid}/cmdline','rb').read().split(b'\0') if x]
    except Exception: continue
    if not argv: continue
    gate=any(os.path.basename(a)=='balance_mod_v020_real_gate.sh' for a in argv)
    reasonix=('serve' in argv and any(a=='deepseek-v20/deepseek-v4-flash' for a in argv))
    if gate or reasonix:
        try: os.kill(pid,signal.SIGTERM); stopped+=1
        except (ProcessLookupError,PermissionError): pass
print(stopped)
PYINNER
)"
sleep 0.25
rm -rf /tmp/reasonix-v020-real-gate.lock 2>/dev/null || true
echo "v0.20 stale online-gate cleanup: PASS (stopped=$STOPPED)"

PATHS=(
  configs/balance_mod_v020_real_provider_manifest.json
  configs/reasonix.balance.v020.real.template.toml
  docs/BALANCE_MOD_V020.md
  docs/BALANCE_MOD_V020_FIXED.md
  docs/BALANCE_MOD_V020_FIXED6.md
  scripts/balance_mod_v020_reconcile.py
  scripts/balance_mod_v020_real_gate.sh
  scripts/balance_mod_v020_targeted.sh
  scripts/balance_mod_smoke_quick_v020.sh
  scripts/balance_mod_v020_preflight.sh
  scripts/balance_mod_smoke_v020.sh
  internal/agent/balance_strict_retry_fixed6_test.go
  internal/provider/balance_strict_retry_fixed6_test.go
)
for p in "${PATHS[@]}"; do
  [[ -f "$PAYLOAD/$p" ]] || { echo "ERROR: payload missing $p" >&2; exit 2; }
done

TMP="$(mktemp -d)"
PATCH="$TMP/v020-fixed6.patch"
BACKUP="$TARGET/.balance_mod_backups/v0.20-fixed6-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$BACKUP"
: > "$PATCH"
trap 'rm -rf "$TMP"' EXIT

mkdir -p "$TMP/core/internal/agent" "$TMP/core/internal/provider" "$TMP/core/internal/provider/openai"
cp internal/agent/run_budget.go "$TMP/core/internal/agent/run_budget.go"
cp internal/agent/sampling_request.go "$TMP/core/internal/agent/sampling_request.go"
cp internal/agent/reasoning_replay.go "$TMP/core/internal/agent/reasoning_replay.go"
cp internal/agent/run_loop.go "$TMP/core/internal/agent/run_loop.go"
cp internal/provider/retry.go "$TMP/core/internal/provider/retry.go"
cp internal/provider/openai/openai.go "$TMP/core/internal/provider/openai/openai.go"

"$PY_BIN" - "$TMP/core" <<'PYCORE'
from pathlib import Path
import re,sys
root=Path(sys.argv[1])

p=root/'internal/agent/run_budget.go'; s=p.read_text()
if 'strict-one-paid-attempt-v0.20-fixed6' not in s:
    old=r'''// strictPreCallRetryFactor reserves enough room for the complete bounded retry
// envelope: body-stream retries multiplied by provider header/status retries.
// It is deliberately conservative: unused reserve costs nothing, but it prevents
// one interrupted request from consuming the entire remaining hard budget and
// then being retried with no room left.
const (
	strictPreCallRetryFactor = maxSamplingAttempts * (provider.MaxRetries + 1)
	// Current Reasonix pricing can represent cache writes as ordinary input-token
	// equivalents up to 2x (for example a long-lived cache write). Pre-call has
	// no usage record yet, so reserve that supported worst case instead of
	// assuming every prompt token is a plain cache miss.
	strictPreCallInputChargeMultiplier = 2.0
)'''
    new=r'''// strict-one-paid-attempt-v0.20-fixed6: strict hard-budget mode disables
// hidden provider/header retries and agent stream/reasoning replays. Therefore
// this guard admits exactly the current paid attempt against the current
// remaining allowance. A later paid attempt must return through host admission
// after the ledger has observed the previous attempt.
const strictPreCallInputChargeMultiplier = 2.0'''
    if old not in s: raise SystemExit('ERROR: expected v0.16 retry-factor block not found in run_budget.go')
    s=s.replace(old,new,1)
    if s.count('strictPreCallRetryFactor') < 2: raise SystemExit('ERROR: strict retry share expressions missing')
    s=s.replace('share := remaining / strictPreCallRetryFactor','share := remaining',1)
    s=s.replace('share := remaining / float64(strictPreCallRetryFactor)','share := remaining',1)
    s=s.replace('retry-reserved share %d <= prompt upper bound %d','current-attempt allowance %d <= prompt upper bound %d',1)
    s=s.replace('retry-reserved share %.6f %s','current-attempt allowance %.6f %s',1)
p.write_text(s)

p=root/'internal/provider/retry.go'; s=p.read_text()
if 'WithRequestRetryLimit' not in s:
    anchor='type requestAttemptCounterKey struct{}\n'
    if anchor not in s: raise SystemExit('ERROR: retry.go requestAttemptCounterKey anchor missing')
    helper=r'''type requestAttemptCounterKey struct{}
type requestRetryLimitKey struct{}

// WithRequestRetryLimit caps SendWithRetry retries for this logical request.
// A value of 0 means exactly one HTTP attempt. The default remains MaxRetries.
func WithRequestRetryLimit(ctx context.Context, max int) context.Context {
	if ctx == nil { ctx = context.Background() }
	if max < 0 { max = 0 }
	if max > MaxRetries { max = MaxRetries }
	return context.WithValue(ctx, requestRetryLimitKey{}, max)
}

func requestRetryLimit(ctx context.Context) int {
	if ctx != nil {
		if max, ok := ctx.Value(requestRetryLimitKey{}).(int); ok {
			if max < 0 { return 0 }
			if max > MaxRetries { return MaxRetries }
			return max
		}
	}
	return MaxRetries
}
'''
    s=s.replace(anchor,helper,1)
    anchor2='func SendWithRetry(ctx context.Context, httpClient *http.Client, opts SendOptions, newReq func(context.Context) (*http.Request, error)) (*http.Response, error) {\n\tnotify := retryNotifyFromContext(ctx)\n'
    if anchor2 not in s: raise SystemExit('ERROR: SendWithRetry anchor missing')
    s=s.replace(anchor2,anchor2+'\tmaxRetries := requestRetryLimit(ctx)\n',1)
    if 'for attempt := 0; attempt <= MaxRetries; attempt++ {' not in s: raise SystemExit('ERROR: SendWithRetry loop missing')
    s=s.replace('for attempt := 0; attempt <= MaxRetries; attempt++ {','for attempt := 0; attempt <= maxRetries; attempt++ {',1)
    s=s.replace('RetryInfo{Attempt: attempt, Max: MaxRetries, Delay: delay, Err: lastErr}','RetryInfo{Attempt: attempt, Max: maxRetries, Delay: delay, Err: lastErr}',1)
p.write_text(s)


# Fixed6 exposes the effective request retry limit so the OpenAI provider can
# choose a single atomic non-stream transport under strict hard-budget mode.
p=root/'internal/provider/retry.go'; s=p.read_text()
if 'func RequestRetryLimit(' not in s:
    anchor='func requestRetryLimit(ctx context.Context) int {'
    pos=s.find(anchor)
    if pos < 0: raise SystemExit('ERROR: Fixed5 requestRetryLimit helper missing')
    # insert exported wrapper after the existing helper function body
    start=pos; brace=s.find('{',pos); depth=0; end=None
    for i in range(brace,len(s)):
        if s[i]=='{': depth+=1
        elif s[i]=='}':
            depth-=1
            if depth==0:
                end=i+1; break
    if end is None: raise SystemExit('ERROR: cannot locate requestRetryLimit end')
    helper='''\n\n// RequestRetryLimit reports the effective header/status retry ceiling carried by ctx.\n// Zero means exactly one HTTP attempt.\nfunc RequestRetryLimit(ctx context.Context) int { return requestRetryLimit(ctx) }'''
    s=s[:end]+helper+s[end:]
p.write_text(s)

p=root/'internal/provider/openai/openai.go'; s=p.read_text()
if 'strict-single-nonstream-v0.20-fixed6' not in s:
    old='''func (c *client) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {\n\tstream, err := c.openStream(ctx, c.chatURL, c.buildRequest(req), req.Tools)'''
    new='''var strictSingleAttemptTimeout = 45 * time.Second\n\nfunc (c *client) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {\n\tif c.deepseek && provider.RequestRetryLimit(ctx) == 0 {\n\t\t// strict-single-nonstream-v0.20-fixed6: a hard-budget call is one atomic\n\t\t// paid HTTP request with complete usage. Avoid SSE/proxy stalls and all\n\t\t// provider prefix/body continuation paths in this mode.\n\t\treturn c.openNonStreamStrict(ctx, c.chatURL, c.buildRequest(req), req.Tools)\n\t}\n\tstream, err := c.openStream(ctx, c.chatURL, c.buildRequest(req), req.Tools)'''
    if old not in s: raise SystemExit('ERROR: openai Stream anchor missing')
    s=s.replace(old,new,1)
    anchor='''func (c *client) openStream(ctx context.Context, targetURL string, wireReq chatRequest, tools []provider.ToolSchema) (<-chan provider.Chunk, error) {'''
    if anchor not in s: raise SystemExit('ERROR: openStream anchor missing')
    helper=r'''func (c *client) openNonStreamStrict(ctx context.Context, targetURL string, wireReq chatRequest, tools []provider.ToolSchema) (<-chan provider.Chunk, error) {
	ctx, cancel := context.WithTimeout(ctx, strictSingleAttemptTimeout)
	defer cancel()
	requestCtx := provider.WithRequestAttemptCounter(ctx)
	wireReq.Stream = false
	wireReq.StreamOptions = nil
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	if err := json.NewEncoder(buf).Encode(wireReq); err != nil {
		bufPool.Put(buf)
		return nil, fmt.Errorf("%s: marshal strict non-stream request: %w", c.name, err)
	}
	body := append([]byte(nil), buf.Bytes()...)
	bufPool.Put(buf)
	newReq := func(ctx context.Context) (*http.Request, error) {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
		if err != nil { return nil, err }
		httpReq.Header.Set("Content-Type", "application/json")
		applyAPIKeyHeader(httpReq.Header, c.baseURL, c.apiKey)
		httpReq.Header.Set("Accept", "application/json")
		applyCustomHeaders(httpReq.Header, c.headers)
		return httpReq, nil
	}
	resp, err := provider.SendWithRetry(requestCtx, c.http, c.sendOpts(), newReq)
	if err != nil { return nil, provider.AnnotateToolSchemaError(err, tools) }
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil { return nil, fmt.Errorf("%s: read strict non-stream response: %w", c.name, err) }
	var nr nonStreamResponse
	if err := json.Unmarshal(raw, &nr); err != nil { return nil, fmt.Errorf("%s: decode strict non-stream response: %w", c.name, err) }
	if nr.Error != nil && strings.TrimSpace(nr.Error.Message) != "" { return nil, fmt.Errorf("%s: %s", c.name, nr.Error.Message) }
	if len(nr.Choices) == 0 { return nil, fmt.Errorf("%s: strict non-stream response has no choices", c.name) }
	c.authed.Store(true)
	choice := nr.Choices[0]
	chunks := make([]provider.Chunk,0,5+2*len(choice.Message.ToolCalls))
	reasoning := choice.Message.ReasoningContent
	if reasoning == "" { reasoning = choice.Message.Reasoning }
	if reasoning != "" { chunks=append(chunks,provider.Chunk{Type:provider.ChunkReasoning,Text:reasoning}) }
	if choice.Message.Content != "" { chunks=append(chunks,provider.Chunk{Type:provider.ChunkText,Text:choice.Message.Content}) }
	for i,tc := range choice.Message.ToolCalls {
		id:=tc.ID; if id=="" { id=fmt.Sprintf("call_%d",i) }
		pt:=&provider.ToolCall{ID:id,Name:tc.Function.Name,Arguments:tc.Function.Arguments}
		if tc.ExtraContent != nil { pt.ThoughtSignature=tc.ExtraContent.Google.ThoughtSignature }
		if pt.ThoughtSignature=="" { pt.ThoughtSignature=tc.Function.ThoughtSignature }
		chunks=append(chunks,provider.Chunk{Type:provider.ChunkToolCallStart,ToolCall:&provider.ToolCall{ID:id,Name:pt.Name}})
		chunks=append(chunks,provider.Chunk{Type:provider.ChunkToolCall,ToolCall:pt})
	}
	if nr.Usage != nil {
		u:=normaliseUsage(nr.Usage); u.FinishReason=choice.FinishReason; provider.ApplyRequestAttemptCount(requestCtx,u)
		chunks=append(chunks,provider.Chunk{Type:provider.ChunkUsage,Usage:u})
	}
	chunks=append(chunks,provider.Chunk{Type:provider.ChunkDone})
	out:=make(chan provider.Chunk,len(chunks)); for _,ch:=range chunks { out<-ch }; close(out)
	return out,nil
}

'''
    s=s.replace(anchor,helper+anchor,1)
    anchor_type='''type streamResponse struct {'''
    if anchor_type not in s: raise SystemExit('ERROR: streamResponse type anchor missing')
    typ=r'''type nonStreamResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			Reasoning string `json:"reasoning"`
			ToolCalls []chatToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *wireUsage `json:"usage"`
	Error *struct { Message string `json:"message"` } `json:"error"`
}

'''
    s=s.replace(anchor_type,typ+anchor_type,1)
p.write_text(s)

p=root/'internal/agent/sampling_request.go'; s=p.read_text()
if 'strict-one-paid-attempt-v0.20-fixed6' not in s:
    old='''func (a *Agent) streamProviderRequest(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {\n\tif err := a.applyStrictPreCallBudget(ctx, &req); err != nil {\n\t\treturn nil, err\n\t}\n\treturn a.svc.prov.Stream(ctx, req)\n}'''
    new='''func (a *Agent) streamProviderRequest(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {\n\tif err := a.applyStrictPreCallBudget(ctx, &req); err != nil {\n\t\treturn nil, err\n\t}\n\tif a.StrictPreCallBudget() {\n\t\t// strict-one-paid-attempt-v0.20-fixed6: no hidden header/status retry\n\t\t// may start another billable HTTP request without a fresh ledger gate.\n\t\tctx = provider.WithRequestRetryLimit(ctx, 0)\n\t}\n\treturn a.svc.prov.Stream(ctx, req)\n}'''
    if old not in s: raise SystemExit('ERROR: v0.16 streamProviderRequest block missing')
    s=s.replace(old,new,1)
    oldcond='if provider.IsStreamInterrupted(result.err) && attempt < maxSamplingAttempts {'
    if oldcond not in s: raise SystemExit('ERROR: stream retry condition missing')
    s=s.replace(oldcond,'if !a.StrictPreCallBudget() && provider.IsStreamInterrupted(result.err) && attempt < maxSamplingAttempts {',1)
p.write_text(s)

p=root/'internal/agent/reasoning_replay.go'; s=p.read_text()
if 'strict-one-paid-attempt-v0.20-fixed6' not in s:
    anchor=') (streamedTurn, bool) {\n\tif !a.activateMissingReasoningFallback() {'
    if anchor not in s: raise SystemExit('ERROR: reasoning fallback anchor missing')
    repl=') (streamedTurn, bool) {\n\tif a.StrictPreCallBudget() {\n\t\t// strict-one-paid-attempt-v0.20-fixed6: fallback would be a second paid call\n\t\t// before the host ledger has admitted another provider attempt.\n\t\treturn streamedTurn{}, false\n\t}\n\tif !a.activateMissingReasoningFallback() {'
    s=s.replace(anchor,repl,1)
p.write_text(s)

p=root/'internal/agent/run_loop.go'; s=p.read_text()
if 'strict-one-paid-attempt-v0.20-fixed6' not in s:
    pat=re.compile(r'(\t\tmissing, shouldRetry := false, false\n\t\tswitch issue \{.*?\n\t\t\}\n)(\t\tif issue == ReasoningReplayOverflow)', re.S)
    m=pat.search(s)
    if not m: raise SystemExit('ERROR: missing-reasoning switch block not found in run_loop.go')
    injected=m.group(1)+'''\t\tif a.StrictPreCallBudget() {\n\t\t\t// strict-one-paid-attempt-v0.20-fixed6: a replay is another paid request.\n\t\t\tshouldRetry = false\n\t\t}\n'''+m.group(2)
    s=s[:m.start()]+injected+s[m.end():]
p.write_text(s)
PYCORE

CORE_PATHS=(
  internal/agent/run_budget.go
  internal/agent/sampling_request.go
  internal/agent/reasoning_replay.go
  internal/agent/run_loop.go
  internal/provider/retry.go
  internal/provider/openai/openai.go
)
for p in "${CORE_PATHS[@]}"; do
  mkdir -p "$BACKUP/$(dirname "$p")"
  cp -a "$p" "$BACKUP/$p"
  rc=0
  diff -u --label "a/$p" --label "b/$p" "$p" "$TMP/core/$p" >> "$PATCH" || rc=$?
  [[ "$rc" == 1 ]] || { echo "ERROR: core diff failed for $p" >&2; exit 4; }
done

for p in "${PATHS[@]}"; do
  mkdir -p "$BACKUP/$(dirname "$p")"
  if [[ -e "$p" ]]; then
    if cmp -s "$p" "$PAYLOAD/$p"; then continue; fi
    cp -a "$p" "$BACKUP/$p"
    rc=0; diff -u --label "a/$p" --label "b/$p" "$p" "$PAYLOAD/$p" >> "$PATCH" || rc=$?
    [[ "$rc" == 1 ]] || { echo "ERROR: diff failed for $p" >&2; exit 4; }
  else
    rc=0; diff -u --label /dev/null --label "b/$p" /dev/null "$PAYLOAD/$p" >> "$PATCH" || rc=$?
    [[ "$rc" == 1 ]] || { echo "ERROR: add-file diff failed for $p" >&2; exit 4; }
  fi
done

if [[ -s "$PATCH" ]]; then
  git apply --check "$PATCH" || { echo 'ERROR: v0.20 Fixed6 exact apply check failed; refusing fuzzy apply' >&2; exit 4; }
  echo 'v0.20 Fixed6 patch apply check: PASS'
  git apply "$PATCH"
else
  echo 'v0.20 Fixed6 already exact'
fi

/usr/local/go/bin/gofmt -w \
  internal/agent/run_budget.go \
  internal/agent/sampling_request.go \
  internal/agent/reasoning_replay.go \
  internal/agent/run_loop.go \
  internal/provider/retry.go \
  internal/agent/balance_strict_retry_fixed6_test.go \
  internal/provider/balance_strict_retry_fixed6_test.go

chmod +x scripts/balance_mod_v020_*.sh scripts/balance_mod_v020_reconcile.py scripts/balance_mod_smoke_*v020.sh
bash -n scripts/balance_mod_v020_real_gate.sh
bash -n scripts/balance_mod_smoke_v020.sh
"$PY_BIN" -m py_compile scripts/balance_mod_v020_reconcile.py
"$PY_BIN" scripts/balance_mod_v020_reconcile.py --self-test >/dev/null
git diff --check

PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local /usr/local/go/bin/go test ./internal/provider \
  -run '^TestBalanceStrictRetryLimitZeroStartsOneHTTPAttempt$' -count=1
PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local /usr/local/go/bin/go test ./internal/agent \
  -run '^(TestBalanceStrictPreCallUsesCurrentAttemptEnvelope|TestBalanceStrictReasoningFallbackSuppressed)$' -count=1
PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local /usr/local/go/bin/go test ./internal/provider/openai \
  -run '^TestBalanceStrictSingleAttempt(UsesNonStreamingDeepSeek|HasHardProviderTimeout)$' -count=1

echo 'v0.20 Fixed6 targeted core policy: PASS'

# Reverse check against pre-gofmt patch can fail only if gofmt changed formatting.
# Build a post-gofmt reverse patch directly from backups for exact validation.
REV="$TMP/v020-fixed6-postgofmt.patch"
: > "$REV"
for p in "${CORE_PATHS[@]}"; do
  rc=0; diff -u --label "a/$p" --label "b/$p" "$BACKUP/$p" "$p" >> "$REV" || rc=$?
  [[ "$rc" == 1 ]] || { echo "ERROR: post-gofmt reverse diff failed for $p" >&2; exit 6; }
done
for p in "${PATHS[@]}"; do
  if [[ -e "$BACKUP/$p" ]]; then
    rc=0; diff -u --label "a/$p" --label "b/$p" "$BACKUP/$p" "$p" >> "$REV" || rc=$?
    [[ "$rc" == 1 || "$rc" == 0 ]] || exit 6
  elif [[ -e "$p" ]]; then
    rc=0; diff -u --label /dev/null --label "b/$p" /dev/null "$p" >> "$REV" || rc=$?
    [[ "$rc" == 1 ]] || exit 6
  fi
done
git apply --reverse --check "$REV" || { echo 'ERROR: v0.20 Fixed6 reverse-apply check failed' >&2; exit 6; }
echo 'v0.20 Fixed6 reverse-apply check: PASS'

echo 'Balance Mod v0.20 Fixed6 applied; diff check: PASS'
echo "Backup: $BACKUP"
echo 'Installer used no API key and made no provider/network call.'
echo "ONLINE FINAL: cd '$TARGET' && PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke_v020.sh"
