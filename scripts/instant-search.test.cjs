const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

// Double DOM kecil ini menguji kontrak event/request, bukan mengklaim QA visual.
function harness() {
 const listeners = {};
 const region = { isConnected:true, querySelector:()=>null, querySelectorAll:()=>[], removeAttribute(){} };
 const document = { addEventListener(){}, body:{addEventListener(name,fn){listeners[name]=fn;}}, querySelector:()=>null };
 const context = vm.createContext({document,location:{origin:'http://localhost',href:'http://localhost/daftar',pathname:'/daftar'},history:{replaceState(){}},FormData:class{constructor(form){this.values=form.values;}has(k){return k in this.values;} *[Symbol.iterator](){yield* Object.entries(this.values);}},URL,DOMParser:class{parseFromString(){return{querySelector:()=>({})};}},setTimeout,clearTimeout,queueMicrotask,htmx:{trigger(){},process(){}},console});
 vm.runInContext(fs.readFileSync('internal/web/static/instant-search.js','utf8'),context);
 return {context,listeners,region};
}

test('GET filter menggabungkan konteks dan kembali ke halaman pertama',()=>{
 const {listeners,region}=harness();
 const second={values:{semester_id:'7',q:'nilai lain'}};
 region.querySelectorAll=()=>[second];
 const form={values:{q:'Ahmad',status:'active'},hasAttribute:()=>true,closest:()=>region};
 const detail={verb:'get',elt:form,headers:{},parameters:{q:'Ahmad',status:'active',page:'9',day_page:'4'}};
 listeners['htmx:configRequest']({detail});
 assert.equal(detail.parameters.q,'Ahmad');assert.equal(detail.parameters.status,'active');assert.equal(detail.parameters.semester_id,'7');assert.equal(detail.parameters.page,'1');assert.equal(detail.parameters.day_page,'1');assert.equal(detail.headers['HX-List-Request'],'true');
});

test('form POST tidak diubah menjadi request pencarian',()=>{
 const {listeners}=harness(); const detail={verb:'post',elt:{closest(){throw Error('POST tidak boleh diproses');}},headers:{}};
 listeners['htmx:configRequest']({detail}); assert.deepEqual(detail.headers,{});
});

test('respons lama ditolak meskipun sukses selama jeda debounce baru',()=>{
 const {context,listeners,region}=harness(); const xhr={status:200,responseText:'hasil lama'};
 context.xhr=xhr;context.region=region;
 vm.runInContext('listRequests.set(xhr,{revision:0,region});listRevision=1;',context);
 const detail={xhr,shouldSwap:true}; listeners['htmx:beforeSwap']({detail});assert.equal(detail.shouldSwap,false);
});

test('modal yang dibuka ketika request berjalan tidak tertutup oleh swap',()=>{
 const {context,listeners,region}=harness();region.querySelector=()=>({open:true});const xhr={status:200};context.xhr=xhr;context.region=region;
 vm.runInContext('listRequests.set(xhr,{revision:0,region});',context);const detail={xhr,shouldSwap:true};listeners['htmx:beforeSwap']({detail});assert.equal(detail.shouldSwap,false);
});

test('isian inline yang belum disimpan melindungi pergantian hasil',()=>{
 const {context,region}=harness();region.querySelectorAll=()=>[{closest:()=>null}];context.region=region;
 assert.equal(vm.runInContext('listProtected(region)',context),true);
});

test('rentang parsial/terbalik tidak mengirim request dan mempertahankan isian',()=>{
 const {context,region}=harness();let sent=0;context.htmx.trigger=()=>sent++;
 const from={value:'2026-10-29'},to={value:'2026-10-01'};
 const form={isConnected:true,checkValidity:()=>true,elements:{namedItem:name=>({from,to})[name]},closest:()=>region};context.form=form;
 vm.runInContext('triggerList(form)',context);assert.equal(sent,0);assert.equal(from.value,'2026-10-29');assert.equal(to.value,'2026-10-01');
 to.value='';vm.runInContext('triggerList(form)',context);assert.equal(sent,0);
 from.value='';vm.runInContext('triggerList(form)',context);assert.equal(sent,1,'rentang opsional kosong boleh mencari');
});

test('ketikan beruntun memakai satu request setelah debounce 350 ms', async()=>{
 const {context,region}=harness();let sent=0;
 context.document.querySelectorAll=()=>[];
 context.htmx.trigger=()=>sent++;
 const form={isConnected:true,hasAttribute:()=>true,checkValidity:()=>true,elements:{namedItem:()=>null},closest:()=>region};
 const input={form,type:'search',name:'q',value:'Ahmad',matches:s=>s.includes('input[type="search"]')};
 context.event={type:'input',target:input};
 vm.runInContext('scheduleList(event);scheduleList(event);scheduleList(event)',context);
 assert.equal(sent,0);
 await new Promise(resolve=>setTimeout(resolve,400));
 assert.equal(sent,1);
});
