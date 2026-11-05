// Regression tests for application logic; no real network or browser history touched.
const {test}=require('node:test'),assert=require('node:assert/strict'),vm=require('node:vm'),fs=require('node:fs'),path=require('node:path');
const html=fs.readFileSync(path.join(__dirname,'../speed_monitor.html'),'utf8');
const source=[...html.matchAll(/<script>([\s\S]*?)<\/script>/g)].map(m=>m[1]).join('\n').replace(/\ninit\(\);\s*$/,'');
function app(){
 const elements=new Map(),storage=new Map(),timeouts=new Map();let tid=0,blob;
 const el=()=>({value:'',style:{},classList:{add(){},remove(){},toggle(){}},addEventListener(){},appendChild(){},insertBefore(){},click(){},parentElement:{style:{}},getBoundingClientRect(){return {bottom:500,left:0}},textContent:''});
 const document={getElementById(id){if(!elements.has(id))elements.set(id,el());return elements.get(id)},querySelectorAll(){return[]},createElement:el,addEventListener(){},removeEventListener(){},body:el()};
 const context={document,navigator:{userAgent:'Chrome/120',platform:'MacIntel'},location:{protocol:'http:'},console,Date,JSON,Math,Number,String,Set,Map,Promise,Error,DOMException,AbortController,performance,
 localStorage:{getItem:k=>storage.get(k)||null,setItem:(k,v)=>storage.set(k,v)},
 setTimeout:(fn,ms)=>{timeouts.set(++tid,{fn,ms});return tid},clearTimeout:id=>timeouts.delete(id),setInterval:()=>++tid,clearInterval(){},alert(){},Blob,
 URL:{createObjectURL:b=>{blob=b;return'blob:test'},revokeObjectURL(){}},fetch:async()=>({arrayBuffer:async()=>new ArrayBuffer(0)})};context.window=context;
 vm.createContext(context);vm.runInContext(source,context);
 return {context,run:code=>vm.runInContext(code,context),storage,timeouts,elements,exported:async()=>await blob.text()};
}
const hdr='timestamp,epoch,type,download_mbps,upload_mbps,ping_ms,machine,browser,iface,result,server';
test('CSV roundtrip preserves zero ping, events, dropouts, quotes/newlines and distinct same-time records',async()=>{
 const a=app();a.run(`logs=[{_t:'data',timestamp:'9/25/26 10:00 PM',epoch:1790398800.123,machine:'A',status:'ok',download_mbps:1000,upload_mbps:35,ping_ms:0,server:'West, US'},
 {_t:'ping',timestamp:'9/25/26 10:01 PM',status:'dropout',ping_ms:'',download_mbps:'',upload_mbps:'',note:'dropout: confirmed'},
 {_t:'evt',timestamp:'9/25/26 10:02 PM',status:'ok',_txt:'PAUSED'},
 {_t:'evt',timestamp:'9/25/26 10:02 PM',status:'ok',_txt:'RESUMED'},
 {_t:'data',timestamp:'9/25/26 10:03 PM',status:'fail',download_mbps:'',upload_mbps:'',ping_ms:'',note:'fail: a, "quote"\\nnext line'}];exportCSV();`);
 const csv=await a.exported();const b=app();b.context.csv=csv;b.run('importCSV(csv);importCSV(csv)');
 assert.equal(b.run('logs.length'),5);assert.equal(b.run('logs.filter(e=>e._t==="evt").length'),2);
 assert.equal(b.run('logs.find(e=>e._t==="ping").status'),'dropout');
 assert.equal(b.run('logs.find(e=>e.status==="ok"&&e._t==="data").ping_ms'),'0');
 assert.match(b.run('logs.find(e=>e.status==="fail").note'),/\nnext line/);
 b.run('exportCSV()');assert.equal(await b.exported(),csv);
});
test('bogus, nonfinite, missing and negative measurements are quarantined on import/load/acquisition',()=>{
 const a=app();a.context.csv=hdr+'\n9/25/26 10:00 PM,1790398800,full,996531233,8334302,20,A,mac,eth,,\n9/25/26 10:01 PM,1790398860,ping,,,2501,A,mac,eth,,';
 a.run('importCSV(csv)');assert.equal(a.run('logs.filter(e=>e.status==="fail").length'),2);
 assert.match(a.run('logs[0].note'),/996531233/);
 a.run('logs=loadLogs()');assert.equal(a.run('logs[0].status'),'fail');
 for(const down of ['NaN','Infinity','-1','2001','0','null']) assert.equal(a.run(`validFullMeasurement({download_mbps:${down},upload_mbps:35,ping_ms:20})`),false);
 assert.equal(a.run('validFullMeasurement({download_mbps:2000,upload_mbps:500,ping_ms:2000})'),true);
});
test('all historical schemas, milliseconds/seconds epochs and event aliases import in chronological order',()=>{
 const a=app();a.context.csv='date,time,machine,os,conn_type,iface_name,status,ping_ms,download_mbps,upload_mbps,note\n9/25/26,10:00 PM,A,mac,eth,,ok,20,1000,35,\n';a.run('importCSV(csv)');
 a.context.csv='timestamp,machine,iface_type,iface_name,status,ping_ms,download_mbps,upload_mbps,note\n9/25/26 9:00 PM,B,eth,,ok,20,900,35,\n';a.run('importCSV(csv)');
 a.context.csv='timestamp,download_mbps,upload_mbps,ping_ms,machine,browser,iface,result\n9/25/26 8:00 PM,800,35,20,C,win,eth,\n';a.run('importCSV(csv)');
 assert.equal(a.run('logs.length'),3);assert.equal(a.run('logs[0].machine'),'C');
 assert.equal(a.run('recordEpoch({epoch:1790398800123})'),1790398800123);
 assert.equal(a.run('recordEpoch({epoch:1790398800.123})'),1790398800123);
 assert.equal(a.run('normalizeRecord({_t:"event",note:"STARTED"})._t'),'evt');
});
test('URL schema has round field, proper quoting and explicit boolean normalization',async()=>{
 const a=app();a.storage.set('speedmon_url_logs_v1',JSON.stringify([{ts:'x',url:'https://x/?a=1,2',state:'timeout',ms:0,conn_ok:'false',round:'confirm'}]));a.run('exportUrlLog()');const csv=await a.exported();
 assert.match(csv,/timestamp,url,state,ms,conn_ok,round/);assert.match(csv,/"https:\/\/x\/\?a=1,2",timeout,0,false,confirm/);
 a.context.csv=csv;a.run('importCSV(csv)');assert.equal(a.run('logs.length'),0);
});
test('availability excludes unknown failures; missing latency is excluded; formatting escapes injection',()=>{
 const a=app();a.run(`logs=[{_t:'data',epoch:Date.now()/1000,status:'ok',download_mbps:1000,upload_mbps:35,ping_ms:''},
 {_t:'data',epoch:Date.now()/1000,status:'ok',download_mbps:900,upload_mbps:35,ping_ms:20},
 {_t:'ping',epoch:Date.now()/1000,status:'dropout'}, {_t:'data',epoch:Date.now()/1000,status:'fail'},
 {_t:'ping',epoch:Date.now()/1000,status:'fail',conn_ok:true}];updateStats();`);
 assert.equal(a.elements.get('avPing').textContent,'20ms');assert.equal(a.elements.get('avUptime').textContent,'75%');
 assert.equal(a.run(`esc('<img title="x">')`),'&lt;img title=&quot;x&quot;&gt;');
});
test('Stop during ping discards completion and cannot rearm schedules; exceptions release guard',async()=>{
 const a=app();let release;a.context.fetch=()=>new Promise(r=>release=r);
 const pending=a.run('runPingCheck()');a.run('document.getElementById("btnStop").onclick()');
 // Each subsequent probe resolves immediately, but the run generation is stale.
 a.context.fetch=async()=>({});release({});await pending;
 assert.equal(a.run('pinging'),false);assert.equal(a.run('nextPingAt'),null);assert.equal(a.run('logs.filter(e=>e._t==="ping").length'),0);
 a.run('paused=false;classifyConnectivity=async()=>{throw Error("probe failed")};timedFetch=async()=>{throw Error("offline")}');await a.run('runPingCheck()');assert.equal(a.run('pinging'),false);
});
test('final NDT client values are used, not peaks or raw byte counters; Stop terminates workers',async()=>{
 const a=app();const workers=[];
 a.context.Worker=class {constructor(){workers.push(this)} addEventListener(){}terminate(){this.terminated=true}};
 a.context.ndt7={discoverServerURLs:async()=>({'///ndt/v7/download':'wss://example.test/download','///ndt/v7/upload':'wss://example.test/upload'}),downloadTest:async(c,cb)=>{new a.context.Worker('download');cb.downloadComplete({LastClientMeasurement:{MeanClientMbps:1000,NumBytes:1250000000},LastServerMeasurement:{TCPInfo:{MinRTT:20000}}});return 0},uploadTest:async(c,cb)=>{new a.context.Worker('upload');cb.uploadComplete({LastClientMeasurement:{MeanClientMbps:35}});return 0}};
 const result=await a.run('runNdt7()');assert.equal(result.down,1000);assert.equal(result.up,35);assert.equal(result.ping,20);assert.ok(workers.every(w=>w.terminated));
 a.context.ndt7.downloadTest=()=>new Promise(resolve=>{const w=new a.context.Worker('download');w.resolve=resolve});
 const pending=a.run('runNdt7()');await new Promise(setImmediate);a.run('cancelActiveWork()');await assert.rejects(pending,{name:'AbortError'});assert.ok(workers.every(w=>w.terminated));
});
test('absolute scheduler keeps future slots, skips missed slots, and honors full interval before ping interval',()=>{
 const a=app();assert.equal(a.run('nextSlot(1000,100,1050)'),1100);assert.equal(a.run('nextSlot(1000,100,1300)'),1400);assert.equal(a.run('nextSlot(2000,100,1000)'),2000);
 a.run('pageScheduling=true;schedulePing(15);scheduleTest(1)');const last=[...a.timeouts.values()].at(-1);assert.ok(last.ms<=60000&&last.ms>59000);
 a.run('paused=true;clearAllTimers();schedulePing(1);scheduleTest(1)');assert.equal(a.timeouts.size,0);
});
test('late cancelled NDT callbacks cannot change UI or start upload',async()=>{
 const a=app();let callbacks,uploads=0;
 a.context.Worker=class {addEventListener(){}terminate(){}};
 a.context.ndt7={discoverServerURLs:async()=>({'///ndt/v7/download':'wss://example.test/download','///ndt/v7/upload':'wss://example.test/upload'}),downloadTest:(c,cb)=>{callbacks=cb;return new Promise(resolve=>{const w=new a.context.Worker('download');w.resolve=resolve})},uploadTest:async()=>{uploads++;return 0}};
 const pending=a.run('runNdt7()');await new Promise(setImmediate);a.run('cancelActiveWork();setStatus("Stopped")');await assert.rejects(pending,{name:'AbortError'});
 callbacks.downloadStart();callbacks.uploadStart();assert.equal(a.elements.get('stxt').textContent,'Stopped');assert.equal(uploads,0);
});

test('shared latency filters invalid readings and handles odd/even medians',()=>{
 const a=app();
 assert.equal(a.run('summarizeLatency([0,NaN,Infinity,2500,50,60,70]).median'),60);
 assert.equal(a.run('summarizeLatency([40,50,60,90,150]).median'),60);
 assert.equal(a.run('summarizeLatency([40,50,60,90,150]).average'),78);
 assert.equal(a.run('summarizeLatency([0,40,50,60,90]).median'),55);
 assert.throws(()=>a.run('summarizeLatency([0,2500,NaN])'));
});
test('shared probe makes ten requests and ignores zero and long samples',async()=>{
 const a=app();a.run('timedFetch=async()=>[0,40,50,60,2500].shift()');
 a.run('let probeIndex=0;timedFetch=async()=>[0,40,50,60,2500,70,80,90,100,110][probeIndex++]');
 const result=await a.run('measureLatency(runGeneration)');
 assert.equal(result.median,75);assert.equal(result.average,75);
 assert.equal(a.run('probeIndex'),10);
});

test('full test displays shared HTTP median rather than NDT minimum RTT',async()=>{
 const a=app();a.run(`measureLatency=async()=>({median:62,average:70,samples:[50,62,98]});
 runNdt7=async()=>({down:420,up:35,ping:12,server:'test'});
 addEntry=e=>logs.push(e);setTesting=()=>{};setProgress=()=>{};updateStats=()=>{};updateIdleStatus=()=>{};`);
 await a.run('runFullTest()');assert.equal(a.run('logs[0].ping_ms'),62);
 assert.equal(a.run('logs[0].download_mbps'),420);
});

test('ten valid samples use the middle pair, and cancellation stops subsequent probes',async()=>{
 const a=app();a.run('let calls=0;timedFetch=async()=>++calls*10');
 const result=await a.run('measureLatency(runGeneration)');
 assert.equal(result.median,55);assert.equal(result.average,55);assert.equal(result.samples.length,10);
 const b=app();b.run('let calls=0;timedFetch=async()=>{calls++;runGeneration++;return 50;}');
 await assert.rejects(b.run('measureLatency(runGeneration)'),/cancelled/);
 assert.equal(b.run('calls'),1);
});

test('rate limiting starts cooldown and prevents subsequent bandwidth requests',async()=>{
 const a=app();a.run(`let bandwidthCalls=0;measureLatency=async()=>({median:60});
 runNdt7=async()=>{bandwidthCalls++;throw new Error('M-Lab rate limited (HTTP 429)');};
 classifyConnectivity=async()=> 'online';addEntry=e=>logs.push(e);
 setTesting=()=>{};setProgress=()=>{};updateStats=()=>{};updateCharts=()=>{};renderNetDisplay=()=>{};updateIdleStatus=()=>{};`);
 await a.run('runFullTest()');
 assert.equal(a.run('logs[0].status'),'fail');assert.match(a.run('logs[0].note'),/429/);
 assert.ok(a.run('fullCooldownUntil>Date.now()'));
 await a.run('runFullTest()');assert.equal(a.run('bandwidthCalls'),1);
});

test('cooldown persists and bandwidth failure exposes its reason',()=>{
 const a=app();a.run('setFullCooldown(Date.now()+1800000);updateIdleStatus()');
 assert.ok(Number(a.storage.get('speedmon_full_cooldown_until'))>Date.now());
 assert.match(a.elements.get('stxt').textContent,/cooldown/);
 const row=a.run("makeRow({_t:'data',status:'fail',timestamp:'now',machine:'A',note:'fail: websocket refused'})");
 assert.match(row.title,/websocket refused/);assert.match(row.innerHTML,/TEST FAIL/);
});

test('missing discovery URLs never start download/upload workers',async()=>{
 const a=app();let starts=0;
 a.context.Worker=class {addEventListener(){}terminate(){}};
 a.context.ndt7={discoverServerURLs:async()=>({}),downloadTest:async()=>{starts++},uploadTest:async()=>{starts++}};
 await assert.rejects(a.run('runNdt7()'),/no valid download URL/);
 assert.equal(starts,0);
});
