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
 const before={trimming:true,sourceFrame:true,trim:{startMS:0,endMS:30000}};
 editor.sync({...before,trim:{startMS:5050000,endMS:5080000}},before);
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
  assert.equal(visual.sourcePreviewStatus.hidden,originSeconds<1);
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
