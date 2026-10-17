import socket, base64, os, json, struct, sys, time, hashlib

HOST = "127.0.0.1"; PORT = 17760
BEARER = sys.argv[1]
AGENT_ID = sys.argv[2] if len(sys.argv) > 2 else "hunter-probe-0001"

key = base64.b64encode(os.urandom(16)).decode()
s = socket.create_connection((HOST, PORT), timeout=15)
req = (
    f"GET /agent/v1/stream HTTP/1.1\r\n"
    f"Host: {HOST}:{PORT}\r\n"
    f"Upgrade: websocket\r\n"
    f"Connection: Upgrade\r\n"
    f"Sec-WebSocket-Key: {key}\r\n"
    f"Sec-WebSocket-Version: 13\r\n"
    f"Sec-WebSocket-Protocol: homer.stream.v1\r\n"
    f"Authorization: Bearer {BEARER}\r\n"
    f"\r\n"
).encode()
s.sendall(req)
buf = b""
while b"\r\n\r\n" not in buf:
    chunk = s.recv(4096)
    if not chunk: break
    buf += chunk
head, _, rest = buf.partition(b"\r\n\r\n")
print("=== HTTP RESPONSE ==="); print(head.decode(errors="replace"))

def send_text(sock, text):
    payload = text.encode()
    n = len(payload)
    hdr = b"\x81"
    if n < 126: hdr += struct.pack("!B", 0x80 | n)
    elif n < 65536: hdr += struct.pack("!BH", 0x80 | 126, n)
    else: hdr += struct.pack("!BQ", 0x80 | 127, n)
    mask = os.urandom(4)
    masked = bytes(b ^ mask[i % 4] for i, b in enumerate(payload))
    sock.sendall(hdr + mask + masked)

def read_frame(sock):
    def rd(n):
        d = b""
        while len(d) < n:
            c = sock.recv(n - len(d))
            if not c: return d
            d += c
        return d
    b0 = rd(1)
    if not b0: return None
    b0 = b0[0]
    opcode = b0 & 0x0F
    b1 = rd(1)[0]
    masked = bool(b1 & 0x80); ln = b1 & 0x7F
    if ln == 126: ln = struct.unpack("!H", rd(2))[0]
    elif ln == 127: ln = struct.unpack("!Q", rd(8))[0]
    mask = rd(4) if masked else None
    data = rd(ln) if ln else b""
    if mask: data = bytes(b ^ mask[i % 4] for i, b in enumerate(data))
    return opcode, data

hello = json.dumps({"t":"req","id":"h1","m":"hello","p":{
    "proto":1,"agentId":AGENT_ID,"hostname":"hunter-probe","version":"0.0.0-hunter",
    "caps":[],"host":None,"tools":None}})
print("=== SEND HELLO ==="); print(hello[:300])
send_text(s, hello)
deadline = time.time() + 12
while time.time() < deadline:
    fr = read_frame(s)
    if fr is None: break
    opcode, data = fr
    if opcode in (0x1, 0x2):
        print("=== FRAME (opcode %d) ===" % opcode)
        try:
            txt = data.decode()
            print(txt[:1500])
            if "agentSecret" in txt:
                print(">>> AGENT SECRET PRESENT IN WELCOME <<<")
        except Exception as e:
            print("(binary)", e)
        break
    if opcode == 0x8:
        print("=== CLOSE FRAME ===", data[:200]); break
s.close()
