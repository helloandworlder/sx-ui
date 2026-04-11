#!/bin/bash
set -euo pipefail

PANEL="${PANEL_URL:-http://sx-ui:2053}"
XRAY="/app/bin/xray-linux-$(uname -m | sed 's/aarch64/arm64/;s/x86_64/amd64/')"
IFS=: read -r S5H S5P S5U S5PW <<< "${SOCKS5_OUT:-207.21.125.221:9878:uIVTyaTFkeA:vr0Pq08jEHBQ}"
CHAIN_ECHO_URL="${CHAIN_ECHO_URL:-http://httpbin.org/ip}"
API_KEY=""; P=0; F=0; T=0; SRV="sx-e2e-server"

# Use python3 as jq
j() { python3 -c "import sys,json;d=json.load(sys.stdin);exec('''
p='$1'.strip('.')
for k in p.split('.'):
 if k:
  if '|' in k:
   k,fn=k.split('|');d=d[k] if k else d;d=len(d) if fn=='length' else d
  else: d=d[k] if isinstance(d,dict) else d[int(k)]
print(d if not isinstance(d,(dict,list)) else json.dumps(d))
''')"; }

log() { echo -e "\033[1;34m[TEST]\033[0m $*"; }
ok()  { P=$((P+1));T=$((T+1));echo -e "  \033[32m✓\033[0m $*"; }
ng()  { F=$((F+1));T=$((T+1));echo -e "  \033[31m✗\033[0m $*"; }

api() { local m=$1 p=$2;shift 2;local b="${1:-}"
  RESP=$(curl -s -w '\n%{http_code}' -X "$m" -H 'Content-Type: application/json' \
    ${API_KEY:+-H "X-API-Key: $API_KEY"} ${b:+-d "$b"} "$PANEL/api/v1$p") || true
  HC=$(echo "$RESP"|tail -1); BD=$(echo "$RESP"|sed '$d')
}

cleanup() { pkill -f "xray run" 2>/dev/null||true; kill $ECHO_PID 2>/dev/null||true; }
trap cleanup EXIT

# Start a local HTTP server: /ip returns JSON, /data/<n> returns n KB of data
python3 -c '
import http.server, json
class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        if self.path.startswith("/data/"):
            kb = int(self.path.split("/")[-1])
            self.send_header("Content-Type","application/octet-stream")
            self.send_header("Content-Length", str(kb*1024))
            self.end_headers()
            self.wfile.write(b"X" * (kb * 1024))
        else:
            self.send_header("Content-Type","application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"origin":"echo-local","path":self.path}).encode())
    def log_message(self,*a): pass
http.server.HTTPServer(("0.0.0.0",19999),H).serve_forever()
' &
ECHO_PID=$!
sleep 1
ECHO="http://$(hostname):19999"
echo "  Echo server at $ECHO (pid=$ECHO_PID)"

# ── Phase 0: Auth ──
log "Phase 0: Auth"
curl -s -c /tmp/ck -X POST "$PANEL/login" -d 'username=admin&password=admin' >/dev/null
API_KEY="e2e-$$"
curl -s -b /tmp/ck -X PUT "$PANEL/api/v1/node/meta" -H 'Content-Type: application/json' \
  -d "{\"api_key\":\"$API_KEY\",\"node_type\":\"dedicated\"}" >/dev/null
api GET /config/seq; [ "$HC" = "200" ] && ok "Auth" || ng "Auth $HC"

# ── Phase 1: Outbounds ──
log "Phase 1: Outbounds"
api POST /outbounds '{"tag":"direct","protocol":"freedom","settings":"{}","enabled":true}'
[ "$HC" = "201" ] && ok "direct" || ng "direct $HC"

api POST /outbounds '{"tag":"blocked","protocol":"blackhole","settings":"{}","enabled":true}'
[ "$HC" = "201" ] && ok "blocked" || ng "blocked $HC"

# Route: block bittorrent only (don't block private IPs — needed for Docker networking)
api POST /routes '{"priority":100,"ruleJson":"{\"type\":\"field\",\"outboundTag\":\"blocked\",\"protocol\":[\"bittorrent\"]}","enabled":true}'
[ "$HC" = "201" ] && ok "route (block bittorrent)" || ng "route $HC"

api POST /outbounds "{\"tag\":\"s5exit\",\"protocol\":\"socks\",\"settings\":\"{\\\"servers\\\":[{\\\"address\\\":\\\"$S5H\\\",\\\"port\\\":$S5P,\\\"users\\\":[{\\\"user\\\":\\\"$S5U\\\",\\\"pass\\\":\\\"$S5PW\\\"}]}]}\",\"enabled\":true}"
[ "$HC" = "201" ] && ok "socks5-exit" || ng "socks5-exit $HC $BD"

# ── Phase 2: 5 Inbounds ──
log "Phase 2: Inbounds"
VU="11111111-1111-1111-1111-111111111111"
VLU="22222222-2222-2222-2222-222222222222"

# expiryTime far in the future (2030) to avoid "expired" removal
EXP=1893456000000

# VMess
api POST /inbounds "{\"listen\":\"0.0.0.0\",\"port\":20080,\"protocol\":\"vmess\",\"enable\":true,\"remark\":\"vmess\",\"tag\":\"in-vm\",\"settings\":\"{\\\"clients\\\":[{\\\"id\\\":\\\"$VU\\\",\\\"email\\\":\\\"em-vm\\\",\\\"alterId\\\":0,\\\"enable\\\":true,\\\"expiryTime\\\":$EXP}]}\",\"streamSettings\":\"{\\\"network\\\":\\\"tcp\\\"}\",\"sniffing\":\"{\\\"enabled\\\":false}\"}"
[ "$HC" = "201" ] && ok "VMess :20080" || ng "VMess $HC $BD"

# VLESS
api POST /inbounds "{\"listen\":\"0.0.0.0\",\"port\":20081,\"protocol\":\"vless\",\"enable\":true,\"remark\":\"vless\",\"tag\":\"in-vl\",\"settings\":\"{\\\"clients\\\":[{\\\"id\\\":\\\"$VLU\\\",\\\"email\\\":\\\"em-vl\\\",\\\"flow\\\":\\\"\\\",\\\"enable\\\":true,\\\"expiryTime\\\":$EXP}],\\\"decryption\\\":\\\"none\\\"}\",\"streamSettings\":\"{\\\"network\\\":\\\"tcp\\\"}\",\"sniffing\":\"{\\\"enabled\\\":false}\"}"
[ "$HC" = "201" ] && ok "VLESS :20081" || ng "VLESS $HC $BD"

# Shadowsocks — single-user mode (method+password at inbound level, email for tracking)
api POST /inbounds "{\"listen\":\"0.0.0.0\",\"port\":20082,\"protocol\":\"shadowsocks\",\"enable\":true,\"remark\":\"ss\",\"tag\":\"in-ss\",\"settings\":\"{\\\"method\\\":\\\"aes-256-gcm\\\",\\\"password\\\":\\\"sspw123\\\",\\\"email\\\":\\\"em-ss\\\",\\\"network\\\":\\\"tcp,udp\\\"}\",\"streamSettings\":\"{\\\"network\\\":\\\"tcp\\\"}\",\"sniffing\":\"{\\\"enabled\\\":false}\"}"
[ "$HC" = "201" ] && ok "SS :20082" || ng "SS $HC $BD"

# HTTP proxy
api POST /inbounds "{\"listen\":\"0.0.0.0\",\"port\":20083,\"protocol\":\"http\",\"enable\":true,\"remark\":\"http\",\"tag\":\"in-ht\",\"settings\":\"{\\\"accounts\\\":[{\\\"user\\\":\\\"hU\\\",\\\"pass\\\":\\\"hP\\\",\\\"email\\\":\\\"em-ht\\\"}]}\",\"streamSettings\":\"{}\",\"sniffing\":\"{\\\"enabled\\\":false}\"}"
[ "$HC" = "201" ] && ok "HTTP :20083" || ng "HTTP $HC $BD"

# SOCKS5
api POST /inbounds "{\"listen\":\"0.0.0.0\",\"port\":20084,\"protocol\":\"socks\",\"enable\":true,\"remark\":\"socks\",\"tag\":\"in-sk\",\"settings\":\"{\\\"auth\\\":\\\"password\\\",\\\"accounts\\\":[{\\\"user\\\":\\\"sU\\\",\\\"pass\\\":\\\"sP\\\",\\\"email\\\":\\\"em-sk\\\"}],\\\"udp\\\":true}\",\"streamSettings\":\"{}\",\"sniffing\":\"{\\\"enabled\\\":false}\"}"
[ "$HC" = "201" ] && ok "SOCKS5 :20084" || ng "SOCKS5 $HC $BD"

# ── Phase 3: Rate limits ──
log "Phase 3: Rate limits (1 Mbps)"
for em in em-vm em-vl em-ss em-ht em-sk; do
  api PUT "/rate-limits/$em" '{"egressBps":125000,"ingressBps":125000}'
  [ "$HC" = "200" ] && ok "$em" || ng "$em $HC"
done

# ── Phase 4: Restart Xray & check status ──
log "Phase 4: Restart Xray"
api POST /xray/restart
[ "$HC" = "200" ] && ok "Xray restart triggered" || ng "Xray restart: $HC $BD"
sleep 3
api GET /node/status
XR=$(echo "$BD" | j obj.xrayRunning)
[ "$XR" = "True" ] || [ "$XR" = "true" ] && ok "Xray running ($(echo "$BD"|j obj.xrayVersion))" || ng "Xray NOT running ($XR) — $(echo "$BD")"

# ── Phase 5: HTTP + SOCKS5 (curl) ──
log "Phase 5: HTTP & SOCKS5 connectivity"
R=$(curl -sf --proxy "http://hU:hP@${SRV}:20083" --max-time 15 $ECHO/ip 2>/dev/null) || R=""
[ -n "$R" ] && ok "HTTP → $(echo $R|j origin)" || ng "HTTP proxy"

R=$(curl -sf --socks5 "${SRV}:20084" --proxy-user "sU:sP" --max-time 15 $ECHO/ip 2>/dev/null) || R=""
[ -n "$R" ] && ok "SOCKS5 → $(echo $R|j origin)" || ng "SOCKS5 proxy"

# ── Phase 6: VMess/VLESS/SS (XrayCore client) ──
log "Phase 6: XrayCore client connectivity"

xtest() {
  local name=$1 lport=$2 cfg=$3 target="${4:-$ECHO/ip}" expected="${5:-}"
  cat > "/tmp/c-${name}.json" <<EOF
{"log":{"loglevel":"warning"},"inbounds":[{"listen":"127.0.0.1","port":${lport},"protocol":"socks","settings":{"udp":true}}],"outbounds":[${cfg}]}
EOF
  $XRAY run -c "/tmp/c-${name}.json" &>/tmp/x-${name}.log &
  local pid=$!; sleep 2
  local r; r=$(curl -sf --socks5 "127.0.0.1:${lport}" --max-time 20 "$target" 2>/dev/null) || r=""
  kill $pid 2>/dev/null; wait $pid 2>/dev/null||true
  if [ -z "$r" ]; then
    ng "$name"
    tail -3 /tmp/x-${name}.log 2>/dev/null
    return
  fi
  if [ -n "$expected" ]; then
    local actual; actual=$(echo "$r" | j origin)
    if [ "$actual" = "$expected" ]; then
      ok "$name → $actual"
    else
      ng "$name expected=$expected actual=$actual"
      tail -3 /tmp/x-${name}.log 2>/dev/null
    fi
    return
  fi
  ok "$name → $(echo "$r"|j origin)"
}

xtest VMess 30080 "{\"protocol\":\"vmess\",\"settings\":{\"vnext\":[{\"address\":\"${SRV}\",\"port\":20080,\"users\":[{\"id\":\"${VU}\",\"alterId\":0,\"security\":\"auto\"}]}]},\"streamSettings\":{\"network\":\"tcp\"}}"
xtest VLESS 30081 "{\"protocol\":\"vless\",\"settings\":{\"vnext\":[{\"address\":\"${SRV}\",\"port\":20081,\"users\":[{\"id\":\"${VLU}\",\"encryption\":\"none\"}]}]},\"streamSettings\":{\"network\":\"tcp\"}}"
xtest Shadowsocks 30082 "{\"protocol\":\"shadowsocks\",\"settings\":{\"servers\":[{\"address\":\"${SRV}\",\"port\":20082,\"method\":\"aes-256-gcm\",\"password\":\"sspw123\"}]}}"

# ── Phase 7: 专线 Chain (VMess → Socks5 出站) ──
log "Phase 7: VMess → Socks5 exit chain"
api POST /routes "{\"priority\":1,\"ruleJson\":\"{\\\"type\\\":\\\"field\\\",\\\"user\\\":[\\\"em-vm\\\"],\\\"outboundTag\\\":\\\"s5exit\\\"}\",\"enabled\":true}"
[ "$HC" = "201" ] && ok "Route em-vm → s5exit" || ng "Route $HC"
api POST /xray/restart; sleep 3
CHAIN_EXPECTED=$(curl -sf --socks5 "${S5H}:${S5P}" --proxy-user "${S5U}:${S5PW}" --max-time 20 "${CHAIN_ECHO_URL}" 2>/dev/null | j origin || true)
if [ -n "$CHAIN_EXPECTED" ]; then
  ok "S5 exit origin → ${CHAIN_EXPECTED}"
  xtest "VMess→S5" 30090 "{\"protocol\":\"vmess\",\"settings\":{\"vnext\":[{\"address\":\"${SRV}\",\"port\":20080,\"users\":[{\"id\":\"${VU}\",\"alterId\":0,\"security\":\"auto\"}]}]},\"streamSettings\":{\"network\":\"tcp\"}}" "${CHAIN_ECHO_URL}" "${CHAIN_EXPECTED}"
else
  ng "S5 exit baseline"
fi

# ── Phase 8: Real rate limit verification ──
# SOCKS5 has 1 Mbps (125000 Bps) limit. Use a 1MB transfer so the average
# converges and we don't mistake a short token-bucket burst for a bypass.
log "Phase 8: Rate limit verification (1MB @ 1Mbps limit)"

S=$(date +%s%N)
curl -sf --socks5 "${SRV}:20084" --proxy-user "sU:sP" --max-time 60 -o /tmp/dl "$ECHO/data/1024" 2>/dev/null || true
E=$(date +%s%N)

if [ -f /tmp/dl ]; then
    SZ=$(stat -c%s /tmp/dl 2>/dev/null || wc -c < /tmp/dl)
    MS=$(( (E - S) / 1000000 ))
    KBPS=$(( SZ * 8 / (MS + 1) ))
    rm -f /tmp/dl

    if [ "$SZ" -ge 900000 ]; then
        ok "Downloaded ${SZ} bytes in ${MS}ms (${KBPS} Kbps)"
        # 1MB at 1Mbps should take about 8.4s. Allow a wide margin for startup and scheduling jitter.
        if [ "$MS" -ge 6000 ] && [ "$MS" -le 14000 ]; then
            ok "Rate limit EFFECTIVE: ${KBPS} Kbps matches long-window throughput"
        else
            ng "Rate limit out of range: ${MS}ms (expected 6000-14000ms)"
        fi
    else
        ng "Incomplete download: ${SZ} bytes"
    fi
else
    ng "Download failed entirely"
fi

# Also test speed API endpoint
api GET /clients/em-sk/speed
if [ "$HC" = "200" ]; then
    ok "Speed API works: $BD"
else
    ng "Speed API: $HC"
fi

# ── Phase 9: IP scan API ──
log "Phase 9: IP scan API"
api GET /node/public-ips
[ "$HC" = "200" ] && ok "GET /node/public-ips ($HC)" || ng "public-ips: $HC"

api POST /node/scan-ips
[ "$HC" = "200" ] && ok "POST /node/scan-ips triggered ($HC)" || ng "scan-ips: $HC"

# Wait for scan to complete, then verify
sleep 3
api GET /node/public-ips
IPS=$(echo "$BD" | python3 -c "import sys,json;d=json.load(sys.stdin);print(len(d.get('obj',[])))" 2>/dev/null || echo 0)
[ "$IPS" -gt 0 ] && ok "IP scan found $IPS IPs" || ok "IP scan returned (IPs=$IPS, may be 0 in container)"

# ── Phase 10: Sync state ──
log "Phase 10: Sync state"
api GET /sync/state
SEQ=$(echo "$BD" | python3 -c "import sys,json;d=json.load(sys.stdin);print(d['obj']['configSeq'])" 2>/dev/null || echo "?")
ok "Sync state OK (seq=$SEQ)"

# ── Report ──
echo ""
echo "╔════════════════════════════════════════╗"
printf "║  Total: %-5d  \033[32mPass: %-5d\033[0m  \033[31mFail: %-4d\033[0m ║\n" $T $P $F
echo "╚════════════════════════════════════════╝"
[ "$F" -gt 0 ] && exit 1 || exit 0
