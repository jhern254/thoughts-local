import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

function fixture() {
  const revoked=[];
  const context=vm.createContext({AbortController,URL:{revokeObjectURL:url=>revoked.push(url)}});
  vm.runInContext(readFileSync(new URL('../static/video-trim.js',import.meta.url),'utf8'),context);
  const visual={sourcePreviewStatus:{hidden:true},connected:true,state:{open:true,editorID:'editor',id:1},stopPreviewPlayback(){},retireVideo(){},discardPreview(){throw new Error('stale discard published');}};
  const editor=vm.runInContext('new ThoughtsVideoTrim(null)',context);editor.visual=visual;editor.editorID='editor';
  return {editor,visual,context,revoked};
}

test('End edits retain the accepted loop without moving its source cursor',()=>{
  const {editor}=fixture();editor.localFile={};editor.source={sourceID:'source'};
  editor.acceptedVideo={};
  editor.showSource=()=>{throw new Error('draft edit sought source');};
  editor.showAccepted=()=>{throw new Error('draft edit replaced preview');};
  const before={trimming:true,sourceFrame:false,trim:{startMS:0,endMS:30000}};
  editor.sync({...before,trim:{startMS:0,endMS:29000}},before);
});

test('source scrubbing coalesces seeks while a seek is in flight',()=>{
  const {editor,visual}=fixture();let seeked;
  const sourceVideo={seeking:true,currentTime:0,duration:60,addEventListener(name,callback){assert.equal(name,'seeked');seeked=callback;}};
  editor.sourceVideo=sourceVideo;visual.previewVideo=sourceVideo;
  editor.showSource(1000);editor.showSource(2500);
  assert.equal(sourceVideo.currentTime,0);
  sourceVideo.seeking=false;editor.seekSource();
  assert.equal(sourceVideo.currentTime,2.5);
  editor.showSource(3000);seeked();
  assert.equal(sourceVideo.currentTime,3);
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

test('file selection opens a local paused source without upload or processing',async()=>{
 const {editor,visual,context}=fixture();const actions=[];
 context.URL.createObjectURL=()=> 'blob:local';
 context.fetch=()=>{throw new Error('selection uploaded source');};
 visual.state.maxVideoSourceBytes=8*1024**3;visual.state.maxVideoSourceDurationMS=7200000;
 visual.releasePreviewVideo=()=>{};visual.sendAction=action=>actions.push(action);
 const file={size:7*1024**3,arrayBuffer(){throw new Error('whole file buffered');}};
 const video={currentTime:0,duration:6900,videoWidth:1920,videoHeight:1080};
 editor.makeVideo=(url,preload)=>{video.preload=preload;return video;};editor.ready=async()=>{};editor.showSource=position=>assert.equal(position,0);
 await editor.selectSource(file,1);
 assert.equal(editor.localFile,file);assert.equal(editor.source,undefined);
 assert.equal(video.preload,'metadata');
 assert.equal(actions[0].action,'sourceSelected');assert.equal(actions[0].durationMS,6900000);
});

test('Start updates seek the existing local source before native staging',()=>{
 const {editor}=fixture();editor.localFile={};const seeks=[];editor.showSource=ms=>seeks.push(ms);
 const before={trimming:true,sourceFrame:true,trim:{startMS:0,endMS:30000}};
 editor.sync({...before,trim:{startMS:5050000,endMS:5080000}},before);
 assert.deepEqual(seeks,[5050000]);assert.equal(editor.source,undefined);
});

test('explicit previews stage once and reuse the verified source',async()=>{
 const {editor,visual}=fixture();let uploads=0,prepares=0;const actions=[];
 visual.state.trim={startMS:1000,endMS:4000};visual.sendAction=action=>actions.push(action);
 editor.localFile={};editor.setStage=()=>{};editor.showAccepted=()=>{};
 editor.uploadSource=async()=>{uploads++;return {sourceID:'verified',durationMS:60000};};
 editor.accepted={previewID:'accepted',trim:visual.state.trim,durationMS:3000};editor.acceptedVideo={};
 editor.request=async()=>{prepares++;return {json:async()=>editor.accepted};};
 await editor.prepare(visual.state);await editor.prepare(visual.state);
 assert.equal(uploads,1);assert.equal(prepares,2);assert.equal(actions.filter(a=>a.action==='sourceVerified').length,1);
});

test('verified shorter duration rejects the cut without moving the requested scene',async()=>{
 const {editor,visual}=fixture();const actions=[];
 const trim={startMS:59000,endMS:60000};visual.state.trim=trim;visual.sendAction=action=>actions.push(action);
 editor.localFile={};editor.setStage=()=>{};
 editor.uploadSource=async()=>({sourceID:'verified',durationMS:59000});
 editor.request=async()=>{throw new Error('invalid cut reached native preparation');};
 await editor.prepare(visual.state);
 assert.deepEqual(visual.state.trim,trim);assert.equal(actions.at(-1).action,'sourceBounds');assert.equal(editor.source.sourceID,'verified');
});

test('a retired upload cannot publish its source into a newer editor',async()=>{
 const {editor,visual}=fixture();let resolve;editor.setStage=()=>{};editor.localFile={};visual.state.trim={startMS:0,endMS:1000};
 editor.uploadSource=()=>new Promise(done=>{resolve=done;});
 const pending=editor.prepare(visual.state);editor.close();
 resolve({sourceID:'retired',durationMS:60000});await pending;
 assert.equal(editor.source,undefined);
});

test('source-relative cuts work with either browser timestamp origin',async()=>{
 for (const originSeconds of [5, 0, 0.083333]) {
  const {editor,visual,context}=fixture();const actions=[];
  context.URL.createObjectURL=()=> 'blob:offset';context.fetch=()=>{throw new Error('selection uploaded source');};
  visual.releasePreviewVideo=()=>{};visual.sendAction=action=>actions.push(action);
  visual.state.maxVideoSourceBytes=8*1024**3;visual.state.maxVideoSourceDurationMS=7200000;
  const video={currentTime:originSeconds,duration:12,videoWidth:64,videoHeight:48,addEventListener(){}};
  editor.makeVideo=()=>video;editor.ready=async()=>{};editor.attach=()=>{visual.previewVideo=video;};
  await editor.selectSource({size:1024},1);
  assert.equal(actions.at(-1).action,'sourceSelected');
  assert.equal(actions.at(-1).durationMS,12000);
  editor.showSource(6250);
  assert.equal(video.currentTime,originSeconds+6.25);
  editor.showSource(10250);
  assert.equal(visual.sourcePreviewStatus.hidden,originSeconds<1);
  editor.close();
  assert.equal(visual.sourcePreviewStatus.hidden,true);
  assert.equal(editor.sourceStartSeconds,0);
 }
});
