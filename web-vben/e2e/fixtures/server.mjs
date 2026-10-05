// Isolated browser UI fixture; NOT the Go backend and never used in production.
import http from 'node:http';
import { readFileSync } from 'node:fs';
const now = '2026-10-05T02:00:00Z';
const user = { id: 2, username: 'Studio user', email: 'user@example.test', role: 'user', status: 'enabled', group_id: 1, used_bytes: 73400320, created_at: now };
const admin = { ...user, id: 1, username: 'Site admin', email: 'admin@example.test', role: 'admin' };
const tokens = new Map();
let tokenId = 9;
let albums = [];
let settings = { site_name:'ImageNest · QA',registration_enabled:true,guest_upload_enabled:false,gallery_enabled:true,api_enabled:true,trash_days:7,guest_group_id:0,default_group_id:1 };
let tokenRows=[];
const page = (items=[], q=new URLSearchParams())=>({items,total:items.length,page:Number(q.get('page')||1),size:Number(q.get('size')||20)});
const server=http.createServer(async(req,res)=>{
 const u=new URL(req.url,'http://localhost');
 if(!u.pathname.startsWith('/api/') && !u.pathname.startsWith('/t/')){
  const proxy=http.request({hostname:'127.0.0.1',port:5173,path:req.url,method:req.method,headers:{...req.headers,host:'127.0.0.1:5173'}},r=>{res.writeHead(r.statusCode,r.headers);r.pipe(res)});
  proxy.on('error',()=>{res.writeHead(502);res.end('Vite is starting')}); req.pipe(proxy);return;
 }
 let body='';for await(const c of req)body+=c;let input={};try{if(body)input=JSON.parse(body)}catch{}
 const send=(data,status=200,code=0,message='ok')=>{res.writeHead(status,{'Content-Type':'application/json','Cache-Control':'no-store'});res.end(JSON.stringify({code,message,data}));};
 let scenario={};try{scenario=JSON.parse(readFileSync(new URL('../scenario.json',import.meta.url),'utf8'))}catch{}
 if(scenario.delay)await new Promise(r=>setTimeout(r,scenario.delay));
 if(scenario.failPath && u.pathname===scenario.failPath)return send(null,503,50001,'fixture error');
 const current=tokens.get((req.headers.authorization||'').replace('Bearer ',''));
 if(u.pathname==='/api/site')return send({site_name:settings.site_name,register_enabled:settings.registration_enabled,gallery_enabled:settings.gallery_enabled});
 if(u.pathname==='/api/auth/captcha')return send({enabled:false,provider:'',site_key:'',version:0});
 if(u.pathname==='/api/auth/login'){
  if(!['user@example.test','admin@example.test'].includes(input.email)||input.password!=='fixture-password-123')return send(null,401,20002,'invalid credentials');
  const who=input.email.startsWith('admin')?admin:user;const token='fixture-'+(++tokenId);tokens.set(token,who);return send({token,user:who,expires_at:'2027-01-01T00:00:00Z'});
 }
 if(u.pathname==='/api/auth/register')return send(user,201);
 if(u.pathname==='/api/gallery')return send(page([],u.searchParams));
 if(!current)return send(null,401,20001,'unauthenticated');
 if(u.pathname.startsWith('/api/admin/')&&current.role!=='admin')return send(null,403,20003,'forbidden');
 if(u.pathname==='/api/auth/me')return send(current);
 if(u.pathname==='/api/auth/logout'){tokens.delete((req.headers.authorization||'').replace('Bearer ',''));return send(null)}
 if(u.pathname==='/api/auth/password')return send(null);
 if(u.pathname==='/api/policies')return send([{id:1,name:'Default storage',webp_mode:'both',link_prefer:'webp'}]);
 if(u.pathname==='/api/images'||u.pathname==='/api/trash')return send(page([],u.searchParams));
 if(u.pathname==='/api/albums'){
  if(req.method==='POST'){const album={id:Date.now(),name:input.name,intro:input.intro||'',is_public:input.is_public||false,cover_image_id:0,image_count:0,cover_thumb_url:'',created_at:now,updated_at:now};albums.push(album);return send(album,201)}
  return send(page(albums,u.searchParams));
 }
 if(/^\/api\/albums\/\d+$/.test(u.pathname)){
  const id=Number(u.pathname.split('/').pop());if(req.method==='DELETE'){albums=albums.filter(x=>x.id!==id);return send(null)}
  const a=albums.find(x=>x.id===id);if(!a)return send(null,404,10001,'not found');Object.assign(a,input);return send(a);
 }
 if(u.pathname==='/api/tokens'){
  if(req.method==='POST'){const info={id:++tokenId,name:input.name,kind:'api',abilities:['*'],expires_at:input.expires_at||null,last_used_at:null,created_at:now};tokenRows.push(info);return send({token:'isolated-fixture-not-a-real-secret',info},201)}
  return send(tokenRows);
 }
 if(/^\/api\/tokens\/\d+$/.test(u.pathname)){tokenRows=tokenRows.filter(x=>x.id!==Number(u.pathname.split('/').pop()));return send(null)}
 if(u.pathname==='/api/admin/users')return send(page([admin,user],u.searchParams));
 if(u.pathname==='/api/admin/images')return send(page([],u.searchParams));
 if(u.pathname==='/api/admin/groups')return send([{id:1,name:'Default',is_default:true,is_guest:false,capacity_bytes:1073741824,max_file_bytes:20971520,allowed_exts:['jpg','png','webp'],upload_per_min:30,default_policy_id:1,policy_ids:[1],user_count:2,created_at:now,updated_at:now}]);
 if(u.pathname==='/api/admin/storages'||u.pathname==='/api/admin/policies')return send([]);
 if(u.pathname==='/api/admin/settings'){if(req.method==='PUT')Object.assign(settings,input);return send(settings)}
 if(u.pathname==='/api/admin/captcha')return send({version:0,enabled:false,active:null,draft:null,configuration_available:false});
 return send(null,404,10001,'fixture endpoint not implemented');
});
server.listen(8088,'127.0.0.1',()=>console.log('Isolated UI fixture: http://127.0.0.1:8088 (not real backend)'));
