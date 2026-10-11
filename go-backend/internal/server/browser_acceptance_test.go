//go:build integration

package server_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
)

// Only the local test server serves this page; its generated credentials never enter artifacts.
func (f *dockerFixture) browserPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	config, _ := json.Marshal(map[string]string{"api": f.baseURL, "basic": base64.StdEncoding.EncodeToString([]byte("platform:" + f.platformHTTP))})
	fmt.Fprintf(w, `<!doctype html><html lang="zh"><meta charset="utf-8"><title>PiGuard 浏览器验收</title>
<style>body{font:16px system-ui;max-width:850px;margin:40px auto;background:#fafafa}button{padding:10px 20px}li{margin:10px 0}.pass{color:#156b36}.fail{color:#ae2020}</style>
<h1>PiGuard 浏览器验收</h1><p>本地软件环境：真实浏览器 → HTTP / Cookie / WebSocket → Docker 后端。</p>
<button id="run">开始检查</button><p id="status">尚未运行；完整检查约需 70 秒。</p><ul id="results"></ul>
<script>
const config=%s;
window.acceptance={status:'pending',checks:[]};
const log=(name)=>{window.acceptance.checks.push(name);const li=document.createElement('li');li.className='pass';li.textContent='通过：'+name;document.querySelector('#results').append(li)};
const must=(condition,message)=>{if(!condition)throw new Error(message)};
const request=(path,options={})=>fetch(config.api+path,{credentials:'include',...options});
const login=async()=>{const r=await request('/api/v1/auth/session',{method:'POST',headers:{Authorization:'Basic '+config.basic}});must(r.status===201,'登录 HTTP '+r.status);return(await r.json()).data};
const connect=()=>new Promise((resolve,reject)=>{const ws=new WebSocket(config.api.replace('http','ws')+'/api/v1/ws');const timer=setTimeout(()=>{ws.close();reject(new Error('WS 连接超时'))},8000);ws.onopen=()=>{clearTimeout(timer);resolve(ws)};ws.onerror=()=>{clearTimeout(timer);reject(new Error('WS 连接失败'))}});
const closed=(ws)=>new Promise((resolve,reject)=>{if(ws.readyState===WebSocket.CLOSED)return resolve();const timer=setTimeout(()=>reject(new Error('WS 未关闭')),5000);ws.addEventListener('close',()=>{clearTimeout(timer);resolve()},{once:true})});
document.querySelector('#run').onclick=async()=>{
document.querySelector('#run').disabled=true;window.acceptance.status='running';document.querySelector('#status').textContent='正在检查…';
try{
let session=await login();log('跨端口 CORS 登录并保存 Cookie');
must((await request('/api/v1/devices')).status===200,'浏览器 Cookie 查询失败');log('浏览器自动携带会话 Cookie');
must((await request('/api/v1/devices',{headers:{Authorization:'Basic '+btoa('platform:wrong')}})).status===401,'错误 Basic 回退 Cookie');log('Authorization 错误不会回退 Cookie');
const body=JSON.stringify({type:'camera.snapshot',params:{quality:80}});
must((await request('/api/v1/devices/car-001/commands',{method:'POST',headers:{'Content-Type':'application/json'},body})).status===403,'无 CSRF 写请求被接受');log('Cookie 写请求缺 CSRF 被拒绝');
const command=await request('/api/v1/devices/car-001/commands',{method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':session.csrf_token},body});must(command.status===202,'合法 CSRF 拍照失败');const id=(await command.json()).data.command_id;
let view;for(let i=0;i<80;i++){view=(await(await request('/api/v1/commands/'+id)).json()).data;if(view.status==='success')break;await new Promise(r=>setTimeout(r,100))};must(view.status==='success','拍照未成功');log('真实浏览器拍照与命令查询');
await new Promise((resolve,reject)=>{const img=new Image();img.onload=()=>{must(img.naturalWidth>0,'图片没有像素');resolve()};img.onerror=()=>reject(new Error('Cookie 图片读取失败'));img.src=config.api+view.result.url;document.body.append(img)});log('img 稳定 URL 自动携 Cookie 并显示 JPEG');
const ws=await connect();log('浏览器 WebSocket 自动携带 Cookie');const logout=await request('/api/v1/auth/session',{method:'DELETE',headers:{'X-CSRF-Token':session.csrf_token}});must(logout.status===204,'退出失败');await closed(ws);must((await request('/api/v1/devices')).status===401,'退出后 Cookie 仍有效');log('退出撤销 Cookie 并关闭关联 WS');
session=await login();const expiring=await connect();document.querySelector('#status').textContent='正在等待固定会话到期…';await new Promise(r=>setTimeout(r,Math.max(0,Date.parse(session.expires_at)-Date.now()+500)));await closed(expiring);must((await request('/api/v1/devices')).status===401,'到期会话仍有效');log('固定会话到期自动关闭 WS 并拒绝请求');
window.acceptance.status='passed';document.querySelector('#status').textContent='全部浏览器检查通过';
}catch(error){window.acceptance.status='failed';window.acceptance.error=error.message;const li=document.createElement('li');li.className='fail';li.textContent='失败：'+error.message;document.querySelector('#results').append(li);document.querySelector('#status').textContent='检查失败'}
};
</script></html>`, config)
}
