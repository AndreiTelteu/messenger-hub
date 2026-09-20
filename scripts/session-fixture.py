#!/usr/bin/env python3
"""Local, account-free WebKit persistence/isolation fixture. No external traffic."""
import base64
from http.server import BaseHTTPRequestHandler, HTTPServer

PAGE = b'''<!doctype html><html><head><title>Session lab</title><meta charset="utf-8"><link rel="icon" href="/favicon.png" type="image/png"></head>
<body style="font:18px sans-serif;max-width:700px;margin:60px auto;padding:20px">
<h1>Session lab</h1><p>Local test page for Messenger Hub.</p>
<label>Session name <input id="name" aria-label="Session name" placeholder="Name this session"></label>
<button onclick="save()">Save session</button><button onclick="location.reload()">Check session</button>
<p><button onclick="document.getElementById('name').value='alpha-session';save()">Save Alpha</button> <button onclick="document.getElementById('name').value='beta-session';save()">Save Beta</button></p><p id="result" role="status"></p><p><a href="/popup" target="_blank">Open authentication popup</a></p>
<label>Upload attachment <input type="file" aria-label="Upload attachment"></label>
<script>
function show(){document.getElementById('result').textContent='Saved session: '+(localStorage.getItem('name')||'empty')+' | Cookie: '+(document.cookie||'empty');}
function save(){let v=document.getElementById('name').value;localStorage.setItem('name',v);document.cookie='session='+encodeURIComponent(v)+';max-age=31536000;path=/;SameSite=Lax';show();}
show();
</script></body></html>'''
ICON = base64.b64decode('iVBORw0KGgoAAAANSUhEUgAAAEAAAABAEAIAAAB1mzrKAAAAIGNIUk0AAHomAACAhAAA+gAAAIDoAAB1MAAA6mAAADqYAAAXcJy6UTwAAAAGYktHRP///////wlY99wAAAAHdElNRQfqCRQSKTWkNxwrAAAAJXRFWHRkYXRlOmNyZWF0ZQAyMDI2LTA5LTIwVDE4OjQxOjUzKzAwOjAwlQ0PNgAAACV0RVh0ZGF0ZTptb2RpZnkAMjAyNi0wOS0yMFQxODo0MTo1MyswMDowMORQt4oAAAAodEVYdGRhdGU6dGltZXN0YW1wADIwMjYtMDktMjBUMTg6NDE6NTMrMDA6MDCzRZZVAAAC3ElEQVR42u3bT0iTcRzH8Y/WJIKisqEWUfMPCEEtlAIP89IhpJV42SE67RZFVBrVtaJySYhRhO1Qh9BDOizEi4ESUqQ0jQ5lrlnBGMM6BEYbsg7fBjtJtN/zfB637+vu9nt+7+fZ82x8LevsDASyWSiScvYCSp0GINMAZBqATAOQaQAyDUCmAcg0AJkGINMAZBqATAOQrbf6Ddx3az4DTRHfJcDb0HIAqKyt2sM+7NUtzScXgOjC1Bww0z55A0idSdRa815lVvwc7aqo2AZ0zAa/As3PW5/ZsmuWmm6baAOGvOHdQCaT/mHulQ1fAS5XxVbgQjB0cS2c6f+uebR1FPDEGuNAT7irG8ik099NvLLhe0BHNLhYXFufT45LrmxTjAVw99XEcmdKcZMPVbm3Fc5YgKaI7zJ1X2wmjxWFMxbAW9eyj7ojNpMnusIZC1DZUFVH3RGbmbrP6RcxMg1ApgHINACZBiDTAGQagEwDkGkAMg1ApgHINACZBiDTAGQagEwDkBkLsBRLxtkHYyeZHSqcsQDR+am31B2xmYxtFc5YgJn2yZvUHbGZTMwVzliA1OmEB5g+OuGn7osNZErO1LCi4Zvw0P7wruK9H8hxyYCiKYYDyMBez8OuW7kzpTjIlf13KNHJs6FClji49/5m4MWGyMHc2JbMDjl/gEXOdHmskHubfMAiDSPzoPksmY5eXSg0MPC/fxuf/bgT6PdfdwPpwO9GO9dtDcv/P8CUueOveoEnX/pCwEpgZYS9HlPWQICXH8b8wEj5IzeQPZxdZq/HLEcHGDs/eAwY3zRcDaAeP9nrsYJDfwt6Wt1/DxhfN7wRwHJxbr1w3BXw+M2deuDdydfN7JXYw0EBHty++gv4lHxfIlsvHBGg98SVLcC3HbEj7JXYj3wP6D507mypbr2gBbjmObUdSPkSSfYWcBG+Cat8Dn0MLR0agEwDkGkAMg1ApgHINACZBiD7A4m8zoyBeJ6kAAAAAElFTkSuQmCC')
class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        if self.path == '/favicon.png':
            body, content_type = ICON, 'image/png'
        else:
            body, content_type = PAGE, 'text/html; charset=utf-8'
        self.send_header('Content-Type', content_type)
        self.end_headers()
        self.wfile.write(body)
if __name__ == '__main__':
    print('Session lab: http://127.0.0.1:18994', flush=True)
    HTTPServer(('127.0.0.1', 18994), Handler).serve_forever()
