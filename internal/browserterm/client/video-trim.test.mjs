import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

function fixture() {
  const revoked=[];
  const context=vm.createContext({AbortController,URL:{revokeObjectURL:url=>revoked.push(url)}});
  vm.runInContext(readFileSync(new URL('../static/video-trim.js',import.meta.url),'utf8'),context);
  const visual={connected:true,state:{open:true,editorID:'editor',id:1},stopPreviewPlayback(){},retireVideo(){},discardPreview(){throw new Error('stale discard published');}};
  const editor=vm.runInContext('new ThoughtsVideoTrim(null)',context);editor.visual=visual;editor.editorID='editor';
  return {editor,visual,context,revoked};
}

test('draft cut edits neither prepare nor stop an accepted loop',()=>{
  const {editor}=fixture();editor.source={sourceID:'source'};
  editor.acceptedVideo={};
  editor.showSource=()=>{throw new Error('draft edit sought source');};
  editor.showAccepted=()=>{throw new Error('draft edit replaced preview');};
  const before={trimming:true,sourceFrame:false,sourcePositionMS:0,trim:{startMS:0,endMS:30000}};
  editor.sync({...before,trim:{startMS:10,endMS:30000}},before);
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
