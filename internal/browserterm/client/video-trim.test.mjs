import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

function fixture() {
  const revoked=[];
  const timers=new Map();
  const context=vm.createContext({AbortController,setTimeout(callback){timers.set(callback,callback);return callback;},clearTimeout(id){timers.delete(id);},URL:{revokeObjectURL:url=>revoked.push(url)}});
  vm.runInContext(readFileSync(new URL('../static/video-trim.js',import.meta.url),'utf8'),context);
  const visual={preview:{hidden:false},sourcePreviewStatus:{hidden:true},connected:true,state:{open:true,editorID:'editor',id:1},queuePreview(){},stopPreviewPlayback(){},retireVideo(){},discardPreview(){throw new Error('stale discard published');}};
  const editor=vm.runInContext('new ThoughtsVideoTrim(null)',context);editor.visual=visual;editor.editorID='editor';
  return {editor,visual,context,revoked,timers};
}

test('End edits preview the last included source frame and retain the accepted loop',()=>{
 const {editor}=fixture();editor.localFile={};editor.source={sourceID:'source'};
 const accepted={};editor.acceptedVideo=accepted;const seeks=[];
 editor.showSource=position=>seeks.push(position);
 editor.showAccepted=()=>{throw new Error('draft edit replaced accepted loop');};
 const before={trimming:true,sourceFrame:false,sourcePreviewMS:0,trim:{startMS:0,endMS:30000}};
 editor.sync({...before,sourceFrame:true,sourcePreviewMS:28999,trim:{startMS:0,endMS:29000}},before);
 assert.deepEqual(seeks,[28999]);
 assert.equal(editor.acceptedVideo,accepted);
});

test('held trim arrows preserve standard terminal repeat metadata',()=>{
 const {editor,visual}=fixture();visual.state.trimming=true;
 const event={type:'keydown',key:'ArrowLeft',repeat:true};
 assert.equal(editor.repeatedArrowSequence(event),'\x1b[1;1:2D');
 assert.equal(editor.repeatedArrowSequence({...event,key:'ArrowRight',shiftKey:true}),'\x1b[1;2:2C');
 for(const change of [{repeat:false},{type:'keyup'},{ctrlKey:true},{altKey:true},{metaKey:true},{key:'a'}]) {
  assert.equal(editor.repeatedArrowSequence({...event,...change}),'');
 }
 visual.state.trimming=false;
 assert.equal(editor.repeatedArrowSequence(event),'');
});

test('source scrubbing stays visible while coalescing seeks',()=>{
  const {editor,visual}=fixture();
  const sourceVideo={hidden:false,seeking:true,currentTime:0,duration:60,};
  editor.sourceVideo=sourceVideo;visual.previewVideo=sourceVideo;
  editor.showSource(1000);editor.showSource(2500);
  assert.equal(sourceVideo.currentTime,0);
  sourceVideo.seeking=false;editor.seekSource();
  assert.equal(sourceVideo.currentTime,2.5);
  assert.equal(sourceVideo.hidden,false);
  editor.showSource(3000);sourceVideo.onseeked();
  assert.equal(sourceVideo.currentTime,3);
  assert.equal(sourceVideo.hidden,false);
});

test('a delayed seek completion cannot strand the latest scrub target',()=>{
  const {editor,visual}=fixture();
  const sourceVideo=Object.assign(new EventTarget(),{hidden:false,seeking:false,currentTime:0,duration:60});
  editor.sourceVideo=sourceVideo;visual.previewVideo=sourceVideo;
  const dispatchSeeked=()=>{
    sourceVideo.dispatchEvent(new Event('seeked'));
    sourceVideo.onseeked?.();
  };
  editor.showSource(1000);
  // The browser clears seeking before delivering its queued seeked event.
  // Another input can therefore start a newer seek before the old event arrives.
  editor.showSource(2000);
  sourceVideo.seeking=true;
  dispatchSeeked();
  editor.showSource(3000);
  assert.equal(sourceVideo.currentTime,2);
  sourceVideo.seeking=false;
  dispatchSeeked();
  assert.equal(sourceVideo.currentTime,3);
  assert.equal(sourceVideo.hidden,false);
});

test('source preview hides the unrelated still and restores it for a prepared loop or close',()=>{
  const {editor,visual}=fixture();
  visual.previewFrameElement={append(){}};
  visual.queuePreview=()=>{};
  const source={remove(){}};
  const prepared={remove(){}};
  editor.sourceVideo=source;
  editor.attach(source);
  assert.equal(source.hidden,false);
  assert.equal(visual.preview.hidden,true);
  editor.attach(prepared);
  assert.equal(visual.preview.hidden,false);
  editor.attach(source);
  editor.close();
  assert.equal(visual.preview.hidden,false);
});

test('late discard completion cannot clear a newer upload',async()=>{
  const {editor,context}=fixture();let finish;
  context.fetch=()=>new Promise(resolve=>{finish=resolve;});
  const discard=editor.discard(1);
  editor.generation++; // A new chooser owns the same still-open Visual page.
  finish({ok:true});await discard;
});

test('closing cancels processing and releases source and accepted URLs',()=>{
  const {editor,visual,revoked}=fixture();
  const controller=new AbortController();editor.controller=controller;
  const source={},accepted={};editor.sourceVideo=source;editor.acceptedVideo=accepted;visual.previewVideo=accepted;
  editor.sourceURL='blob:source';editor.acceptedVideoURL='blob:loop';editor.acceptedPosterURL='blob:poster';
  editor.close();
  assert.equal(controller.signal.aborted,true);
  assert.equal(visual.previewVideo,undefined);
  assert.equal(editor.accepted,undefined);
  assert.deepEqual(revoked,['blob:source','blob:loop','blob:poster']);
});

for (const preferences of [{reducedMotion:true,userPaused:false},{reducedMotion:false,userPaused:true}]) {
  test(`automatic prepared preview stays still for ${JSON.stringify(preferences)}`,()=>{
    const {editor,visual}=fixture();let plays=0;
    visual.motionQuery={matches:preferences.reducedMotion};
    visual.state.userPaused=preferences.userPaused;
    visual.togglePreviewPlayback=()=>{plays++;};
    visual.preview={};editor.acceptedPosterURL='blob:poster';editor.acceptedVideo={};
    editor.attach=()=>{};
    editor.showAccepted(true);
    assert.equal(plays,0);
    assert.equal(visual.preview.src,'blob:poster');
    assert.ok(editor.acceptedVideo);
  });
}

test('unpreviewed quality edits preserve the acknowledged loop and never prepare automatically',()=>{
 const {editor}=fixture();editor.localFile={};editor.source={sourceID:'source'};
 editor.accepted={previewID:'accepted',videoQuality:'efficient'};
 editor.showSource=()=>{throw new Error('quality edit sought source');};
 editor.showAccepted=()=>{throw new Error('quality edit replaced preview');};
 const before={trimming:true,sourceFrame:false,videoQuality:'efficient'};
 editor.sync({...before,videoQuality:'maximum'},before);
 assert.equal(editor.accepted.videoQuality,'efficient');
 const request=JSON.parse(editor.applyRequest({videoQuality:'maximum',darkness:42,speedPercent:50}).body);
 assert.equal(request.previewID,'accepted');
 assert.equal(request.videoQuality,undefined); // Native accepted metadata owns persistence.
 assert.equal(request.speedPercent,50);
});

test('file selection waits for native validation before opening the cut editor',async()=>{
 const {editor,visual,context}=fixture();const actions=[];let finishValidation, validationStarted;
 const started=new Promise(resolve=>{validationStarted=resolve;});
 context.URL.createObjectURL=()=> 'blob:local';
 visual.state.maxVideoSourceBytes=8*1024**3;visual.state.maxVideoSourceDurationMS=14400000;
 visual.releasePreviewVideo=()=>{};visual.sendAction=action=>actions.push(action);
 const file={size:102900000,arrayBuffer(){throw new Error('whole file buffered');}};
 const video={currentTime:0,duration:1020,videoWidth:1920,videoHeight:1080};
 editor.makeVideo=()=>video;editor.ready=async()=>{};editor.showSource=()=>{};editor.setStage=()=>{};
 editor.uploadSource=()=>new Promise(resolve=>{finishValidation=resolve;validationStarted();});
 const selection=editor.selectSource(file,1);
 await started;
 assert.equal(actions.at(-1)?.action,'sourceChecking');
 assert.equal(actions.some(action=>action.action==='sourceSelected'),false);
 finishValidation({sourceID:'verified',durationMS:1019999,width:1920,height:1080});
 await selection;
 assert.equal(editor.localFile,file);
 assert.equal(editor.source.sourceID,'verified');
 assert.equal(actions.at(-1).action,'sourceSelected');
 assert.equal(actions.at(-1).durationMS,1019999);
});

test('rejected source never opens the cut editor and releases its local file',async()=>{
 const {editor,visual,context,revoked}=fixture();const actions=[];
 context.URL.createObjectURL=()=> 'blob:rejected';
 visual.state.maxVideoSourceBytes=8*1024**3;visual.state.maxVideoSourceDurationMS=14400000;
 visual.releasePreviewVideo=()=>{};visual.sendAction=action=>actions.push(action);
 editor.makeVideo=()=>({currentTime:0,duration:1020,videoWidth:1920,videoHeight:1080});
 editor.ready=async()=>{};editor.showSource=()=>{};editor.setStage=()=>{};
 const rejection=Object.assign(new Error('fixed rejection'),{videoAction:'sourceCodec'});
 editor.uploadSource=async()=>{throw rejection;};
 await assert.rejects(editor.selectSource({size:102900000},1),error=>error===rejection);
 assert.equal(actions.some(action=>action.action==='sourceSelected'),false);
 assert.equal(editor.localFile,undefined);
 assert.deepEqual(revoked,['blob:rejected']);
});

test('Start updates seek the existing local source without repeating native staging',()=>{
 const {editor}=fixture();editor.localFile={};const seeks=[];editor.showSource=ms=>seeks.push(ms);
 const before={trimming:true,sourceFrame:true,sourcePreviewMS:0,trim:{startMS:0,endMS:30000}};
 editor.sync({...before,sourcePreviewMS:5050000,trim:{startMS:5050000,endMS:5080000}},before);
 assert.deepEqual(seeks,[5050000]);assert.equal(editor.source,undefined);
});

test('explicit previews reuse the source validated at selection',async()=>{
 const {editor,visual}=fixture();let prepares=0;const actions=[];
 visual.state.trim={startMS:1000,endMS:4000};visual.sendAction=action=>actions.push(action);
 editor.source={sourceID:'verified',durationMS:60000};editor.localFile={};editor.setStage=()=>{};editor.showAccepted=()=>{};
 editor.uploadSource=()=>{throw new Error('source uploaded twice');};
 editor.accepted={previewID:'accepted',trim:visual.state.trim,durationMS:3000};editor.acceptedVideo={};
 editor.request=async()=>{prepares++;return {json:async()=>editor.accepted};};
 await editor.prepare(visual.state);await editor.prepare(visual.state);
 assert.equal(prepares,2);
 assert.equal(actions.filter(action=>action.action==='loopReady').length,2);
});

test('a retired selection cannot publish its source into a newer editor',async()=>{
 const {editor,visual,context}=fixture();let finishValidation, validationStarted;
 const started=new Promise(resolve=>{validationStarted=resolve;});
 context.URL.createObjectURL=()=> 'blob:retired';
 visual.releasePreviewVideo=()=>{};visual.sendAction=action=>{assert.notEqual(action.action,'sourceSelected');};
 editor.makeVideo=()=>({currentTime:0,duration:60,videoWidth:64,videoHeight:48});
 editor.ready=async()=>{};editor.setStage=()=>{};
 editor.uploadSource=()=>new Promise(resolve=>{finishValidation=resolve;validationStarted();});
 const selection=editor.selectSource({size:1024},1);await started;editor.close();
 finishValidation({sourceID:'retired',durationMS:60000});await selection;
 assert.equal(editor.source,undefined);
});

test('source-relative cuts work with either browser timestamp origin',async()=>{
 for (const originSeconds of [5, 0, 0.083333]) {
  const {editor,visual,context}=fixture();const actions=[];
  context.URL.createObjectURL=()=> 'blob:offset';context.fetch=()=>{throw new Error('selection uploaded source');};
  visual.releasePreviewVideo=()=>{};visual.sendAction=action=>actions.push(action);
  visual.state.maxVideoSourceBytes=8*1024**3;visual.state.maxVideoSourceDurationMS=7200000;
  const video={currentTime:originSeconds,duration:12,videoWidth:64,videoHeight:48,addEventListener(){}};
  editor.uploadSource=async()=>({sourceID:'verified',durationMS:12000,width:64,height:48});editor.setStage=()=>{};
  editor.makeVideo=()=>video;editor.ready=async()=>{};editor.attach=()=>{visual.previewVideo=video;};
  await editor.selectSource({size:1024},1);
  assert.equal(actions.at(-1).action,'sourceSelected');
  assert.equal(actions.at(-1).durationMS,12000);
  editor.showSource(6250);
  assert.equal(video.currentTime,originSeconds+6.25);
  editor.showSource(10250);
  assert.equal(editor.sourcePreviewMode,originSeconds<1 ? 'browser' : 'native');
  editor.close();
  assert.equal(visual.sourcePreviewStatus.hidden,true);
  assert.equal(editor.sourceStartSeconds,0);
 }
});

test('rejection actions use only fixed reasons rather than response text',()=>{
 const {editor}=fixture();
 assert.equal(editor.failureAction(400,'sourceCodec'),'sourceCodec');
 assert.equal(editor.failureAction(400,'PRIVATE decoder path'),'sourceUnsupported');
 assert.equal(editor.failureAction(500,'sourceCodec'),'failed');
 assert.equal(editor.failureAction(413,null),'sourceSize');
 assert.equal(editor.failureAction(503,null),'sourceTools');
});

test('native-compatible source opens even when browser decoding fails',async()=>{
 const {editor,visual,context}=fixture();const actions=[];
 context.URL.createObjectURL=()=> 'blob:local';
 visual.state.maxVideoSourceBytes=8*1024**3;
 visual.releasePreviewVideo=()=>{};visual.sendAction=action=>actions.push(action);
 editor.makeVideo=()=>({});editor.ready=async()=>{throw new Error('unsupported browser container');};
 editor.uploadSource=async()=>({sourceID:'verified-mkv',durationMS:3000,width:160,height:90,browserPreview:true});
 const positions=[];editor.showSource=position=>positions.push(position);
 await editor.selectSource({size:100},1);
 assert.equal(editor.source.sourceID,'verified-mkv');
 assert.equal(editor.sourceVideo,undefined);
 assert.equal(editor.sourcePreviewMode,'native');
 assert.equal(actions.at(-1).action,'sourceSelected');
 assert.deepEqual(positions,[0]);
});

test('native frame requests coalesce and never overlap or publish a retired target',async()=>{
 const {editor,visual,context,timers}=fixture();const requests=[];const revoked=[];
 editor.localFile={};editor.source={sourceID:'source',width:160,height:90};editor.sourcePreviewMode='native';
 context.URL.createObjectURL=blob=>`blob:${blob.position}`;context.URL.revokeObjectURL=url=>revoked.push(url);
 context.Image=class{async decode(){}};
 editor.request=async(path,options)=>new Promise(resolve=>requests.push({input:JSON.parse(options.body),resolve}));
 editor.showSource(100);clearTimers();const first=editor.extractSourceFrame();
 editor.showSource(200);editor.showSource(300);clearTimers();await editor.extractSourceFrame();
 assert.equal(requests.length,1);
 requests[0].resolve({blob:async()=>({position:100})});await first;
 assert.notEqual(visual.preview.src,'blob:100');
 clearTimers();const latest=editor.extractSourceFrame();assert.equal(requests.length,2);assert.equal(requests[1].input.positionMS,300);
 requests[1].resolve({blob:async()=>({position:300})});await latest;
 assert.equal(visual.preview.src,'blob:300');assert.deepEqual(revoked,['blob:100']);
 editor.showSource(400);clearTimers();const retired=editor.extractSourceFrame();editor.close();
 requests[2].resolve({blob:async()=>({position:400})});await retired;
 assert.notEqual(visual.preview.src,'blob:400');assert.ok(revoked.includes('blob:300'));assert.ok(revoked.includes('blob:400'));
 function clearTimers(){for(const timer of timers.keys())context.clearTimeout(timer);}
});
