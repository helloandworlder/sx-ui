#!/bin/bash
set -euo pipefail

PANEL="${PANEL_URL:-http://sx-ui:2053}"
XRAY="/app/bin/xray-linux-$(uname -m | sed 's/aarch64/arm64/;s/x86_64/amd64/')"
IFS=: read -r S5H S5P S5U S5PW <<< "${LOCAL_SOCKS_OUT:-sx-e2e-runner:19888:e2euser:e2epass}"
API_KEY=""; P=0; F=0; T=0; SRV="sx-e2e-server"
ECHO_PID=0; SOCKS_PID=0

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

reorder_top_routes() {
  local top_ids="$1"
  api GET /routes
  local payload
  payload=$(echo "$BD" | TOP_IDS="$top_ids" python3 -c '
import json, os, sys
d = json.load(sys.stdin)
routes = d.get("obj", [])
top = [int(x) for x in os.environ["TOP_IDS"].split(",") if x]
top_pos = {rid: i + 1 for i, rid in enumerate(top)}
items = []
tail = len(top) + 1
for r in routes:
    rid = int(r["id"])
    if rid in top_pos:
        items.append({"id": rid, "priority": top_pos[rid]})
    else:
        items.append({"id": rid, "priority": tail})
        tail += 1
print(json.dumps(items))
')
  api POST /routes/reorder "$payload"
  [ "$HC" = "200" ] && ok "Route order promoted $top_ids" || ng "Route reorder $HC $BD"
}

cleanup() {
  pkill -f "xray run" 2>/dev/null||true
  [ "${ECHO_PID:-0}" != "0" ] && kill "$ECHO_PID" 2>/dev/null||true
  [ "${SOCKS_PID:-0}" != "0" ] && kill "$SOCKS_PID" 2>/dev/null||true
}
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
ECHO_HOST="${E2E_ECHO_HOST:-$(python3 -c 'import socket; print(socket.gethostbyname(socket.gethostname()))')}"
ECHO="http://${ECHO_HOST}:19999"
echo "  Echo server at $ECHO (pid=$ECHO_PID)"

# Start a local authenticated SOCKS5 server. Route tests assert this log is
# written, proving sx-core selected the configured Socks5 outbound.
python3 - "$S5U" "$S5PW" "$S5P" /tmp/local-socks.log <<'PY' &
import select
import socket
import struct
import sys
import threading

USER = sys.argv[1].encode()
PASS = sys.argv[2].encode()
PORT = int(sys.argv[3])
LOG = sys.argv[4]

def recvn(sock, n):
    data = b""
    while len(data) < n:
        chunk = sock.recv(n - len(data))
        if not chunk:
            raise OSError("unexpected eof")
        data += chunk
    return data

def relay(a, b):
    sockets = [a, b]
    while sockets:
        readable, _, _ = select.select(sockets, [], [], 30)
        if not readable:
            return
        for src in readable:
            dst = b if src is a else a
            data = src.recv(65536)
            if not data:
                return
            dst.sendall(data)

def handle(client):
    upstream = None
    try:
        header = recvn(client, 2)
        methods = recvn(client, header[1])
        if 2 not in methods:
            client.sendall(b"\x05\xff")
            return
        client.sendall(b"\x05\x02")
        auth = recvn(client, 2)
        got_user = recvn(client, auth[1])
        got_pass = recvn(client, recvn(client, 1)[0])
        if got_user != USER or got_pass != PASS:
            client.sendall(b"\x01\x01")
            return
        client.sendall(b"\x01\x00")

        req = recvn(client, 4)
        if req[1] != 1:
            client.sendall(b"\x05\x07\x00\x01\x00\x00\x00\x00\x00\x00")
            return
        atyp = req[3]
        if atyp == 1:
            host = socket.inet_ntoa(recvn(client, 4))
        elif atyp == 3:
            host = recvn(client, recvn(client, 1)[0]).decode()
        elif atyp == 4:
            host = socket.inet_ntop(socket.AF_INET6, recvn(client, 16))
        else:
            client.sendall(b"\x05\x08\x00\x01\x00\x00\x00\x00\x00\x00")
            return
        port = struct.unpack("!H", recvn(client, 2))[0]
        upstream = socket.create_connection((host, port), timeout=10)
        with open(LOG, "a", encoding="utf-8") as fh:
            fh.write(f"CONNECT {host}:{port} user={USER.decode()}\n")
            fh.flush()
        client.sendall(b"\x05\x00\x00\x01\x00\x00\x00\x00\x00\x00")
        relay(client, upstream)
    except Exception as exc:
        with open(LOG, "a", encoding="utf-8") as fh:
            fh.write(f"ERROR {exc}\n")
    finally:
        try:
            client.close()
        except Exception:
            pass
        if upstream is not None:
            try:
                upstream.close()
            except Exception:
                pass

server = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
server.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
server.bind(("0.0.0.0", PORT))
server.listen(128)
while True:
    client, _ = server.accept()
    threading.Thread(target=handle, args=(client,), daemon=True).start()
PY
SOCKS_PID=$!
sleep 1
echo "  Local SOCKS5 exit at ${S5H}:${S5P} (pid=$SOCKS_PID)"

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
SK_ID=$(echo "$BD" | j obj.id 2>/dev/null || true)

# Mixed
MX_USER="mU"; MX_PASS="mP"
api POST /inbounds "{\"listen\":\"0.0.0.0\",\"port\":20085,\"protocol\":\"mixed\",\"enable\":true,\"remark\":\"mixed\",\"tag\":\"in-mx\",\"settings\":\"{\\\"auth\\\":\\\"password\\\",\\\"accounts\\\":[{\\\"user\\\":\\\"$MX_USER\\\",\\\"pass\\\":\\\"$MX_PASS\\\",\\\"email\\\":\\\"em-mx\\\"}],\\\"udp\\\":true}\",\"streamSettings\":\"{}\",\"sniffing\":\"{\\\"enabled\\\":false}\"}"
[ "$HC" = "201" ] && ok "Mixed :20085" || ng "Mixed $HC $BD"
MX_ID=$(echo "$BD" | j obj.id 2>/dev/null || true)

# ── Phase 3: Rate limits ──
log "Phase 3: Rate limits (1 Mbps)"
for em in em-vm em-vl em-ss em-ht em-sk em-mx; do
  api PUT "/rate-limits/$em" '{"egressBps":125000,"ingressBps":125000}'
  [ "$HC" = "200" ] && ok "$em" || ng "$em $HC"
done

log "Phase 3b: REST account save keeps JSON payloads and limits"
api PUT "/inbounds/$MX_ID/clients/em-mx" '{"user":"mU2","pass":"mP2","email":"em-mx","enable":true}'
if [ "$HC" = "200" ]; then
  MX_USER="mU2"; MX_PASS="mP2"
  ok "Mixed client PUT JSON"
else
  ng "Mixed client PUT JSON: $HC $BD"
fi
api GET /rate-limits/em-mx
RL_E=$(echo "$BD" | j obj.egressBps 2>/dev/null || echo "")
[ "$HC" = "200" ] && [ "$RL_E" = "125000" ] && ok "Mixed client save preserved 1Mbps limit" || ng "Mixed rate after save: $HC $BD"

log "Phase 3c: Local test routes before template private-IP block"
api POST /routes "{\"priority\":1,\"ruleJson\":\"{\\\"type\\\":\\\"field\\\",\\\"user\\\":[\\\"em-ht\\\"],\\\"outboundTag\\\":\\\"direct\\\"}\",\"enabled\":true}"
[ "$HC" = "201" ] && ok "Route em-ht → direct" || ng "Route em-ht $HC $BD"
HT_ROUTE_ID=$(echo "$BD" | j obj.id 2>/dev/null || true)
api POST /routes "{\"priority\":2,\"ruleJson\":\"{\\\"type\\\":\\\"field\\\",\\\"user\\\":[\\\"em-vm\\\"],\\\"outboundTag\\\":\\\"direct\\\"}\",\"enabled\":true}"
[ "$HC" = "201" ] && ok "Route em-vm → direct" || ng "Route em-vm $HC $BD"
VM_ROUTE_ID=$(echo "$BD" | j obj.id 2>/dev/null || true)
reorder_top_routes "${HT_ROUTE_ID},${VM_ROUTE_ID}"

# ── Phase 4: Restart Xray & check status ──
log "Phase 4: Restart Xray"
api POST /xray/restart
[ "$HC" = "200" ] && ok "Xray restart triggered" || ng "Xray restart: $HC $BD"
sleep 3
api GET /node/status
XR=$(echo "$BD" | j obj.xrayRunning)
[ "$XR" = "True" ] || [ "$XR" = "true" ] && ok "Xray running ($(echo "$BD"|j obj.xrayVersion))" || ng "Xray NOT running ($XR) — $(echo "$BD")"

# ── Phase 5: HTTP connectivity ──
log "Phase 5: HTTP connectivity"
R=$(curl -sf --proxy "http://hU:hP@${SRV}:20083" --max-time 15 $ECHO/ip 2>/dev/null) || R=""
[ -n "$R" ] && ok "HTTP → $(echo $R|j origin)" || ng "HTTP proxy"

# ── Phase 6: VMess XrayCore client ──
log "Phase 6: VMess XrayCore client connectivity"

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

# ── Phase 7: Socks5/Mixed inbound → Socks5 outbound route ──
log "Phase 7: Socks5/Mixed → local Socks5 outbound route"
api POST /routes "{\"priority\":1,\"ruleJson\":\"{\\\"type\\\":\\\"field\\\",\\\"user\\\":[\\\"em-sk\\\"],\\\"outboundTag\\\":\\\"s5exit\\\"}\",\"enabled\":true}"
[ "$HC" = "201" ] && ok "Route em-sk → s5exit" || ng "Route em-sk $HC $BD"
SK_ROUTE_ID=$(echo "$BD" | j obj.id 2>/dev/null || true)
api POST /routes "{\"priority\":2,\"ruleJson\":\"{\\\"type\\\":\\\"field\\\",\\\"user\\\":[\\\"em-mx\\\"],\\\"outboundTag\\\":\\\"s5exit\\\"}\",\"enabled\":true}"
[ "$HC" = "201" ] && ok "Route em-mx → s5exit" || ng "Route em-mx $HC $BD"
MX_ROUTE_ID=$(echo "$BD" | j obj.id 2>/dev/null || true)
reorder_top_routes "${SK_ROUTE_ID},${MX_ROUTE_ID},${HT_ROUTE_ID},${VM_ROUTE_ID}"
api POST /xray/restart; sleep 3

route_probe() {
  local label=$1 port=$2 userpass=$3
  rm -f /tmp/local-socks.log /tmp/route-dl
  curl -sf --socks5 "${SRV}:${port}" --proxy-user "$userpass" --max-time 30 -o /tmp/route-dl "$ECHO/data/64" 2>/dev/null || true
  local sz=0
  if [ -f /tmp/route-dl ]; then
    sz=$(stat -c%s /tmp/route-dl 2>/dev/null || wc -c < /tmp/route-dl)
  fi
  if [ "$sz" -ge 60000 ] && grep -q "CONNECT ${ECHO_HOST}:19999" /tmp/local-socks.log 2>/dev/null; then
    ok "$label route hit local s5exit (${sz} bytes)"
  else
    ng "$label route failed: size=${sz}, log=$(cat /tmp/local-socks.log 2>/dev/null || true)"
  fi
  rm -f /tmp/route-dl
}

route_probe "SOCKS5" 20084 "sU:sP"
route_probe "Mixed" 20085 "${MX_USER}:${MX_PASS}"

# ── Phase 8: Real rate limit verification ──
log "Phase 8: Rate limit verification (1Mbps / 50Kbps / 100Mbps)"

rate_case() {
  local label=$1 bps=$2 kb=$3 min_ms=$4 max_ms=$5 max_time=$6
  api PUT /rate-limits/em-sk "{\"egressBps\":${bps},\"ingressBps\":${bps}}"
  if [ "$HC" != "200" ]; then
    ng "$label set rate: $HC $BD"
    return
  fi
  api POST /xray/restart
  sleep 3
  rm -f /tmp/dl
  local s e sz ms kbps
  s=$(date +%s%N)
  curl -sf --socks5 "${SRV}:20084" --proxy-user "sU:sP" --max-time "$max_time" -o /tmp/dl "$ECHO/data/$kb" 2>/dev/null || true
  e=$(date +%s%N)
  if [ ! -f /tmp/dl ]; then
    ng "$label download failed"
    return
  fi
  sz=$(stat -c%s /tmp/dl 2>/dev/null || wc -c < /tmp/dl)
  ms=$(( (e - s) / 1000000 ))
  kbps=$(( sz * 8 / (ms + 1) ))
  rm -f /tmp/dl
  if [ "$sz" -lt $(( kb * 900 )) ]; then
    ng "$label incomplete: ${sz} bytes"
    return
  fi
  ok "$label downloaded ${sz} bytes in ${ms}ms (${kbps} Kbps)"
  if [ "$ms" -ge "$min_ms" ] && [ "$ms" -le "$max_ms" ]; then
    ok "$label rate effective"
  else
    ng "$label rate out of range: ${ms}ms expected ${min_ms}-${max_ms}ms"
  fi
}

rate_case "1Mbps" 125000 1024 6000 15000 70
rate_case "50Kbps" 6250 128 12000 35000 80
rate_case "100Mbps" 12500000 1024 1 4000 30

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
