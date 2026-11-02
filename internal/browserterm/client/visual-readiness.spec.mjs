import {test,expect} from '@playwright/test';
import {readFileSync} from 'node:fs';
import {openVisual,clickControl,chooseImage,screen} from './visual-helpers.mjs';

test('Visual schema recovery and failed Apply preserve the draft and accepted preview',async({page},testInfo)=>{
 let incompatible=true, rejectApply=true, sockets=0, uploads=0;
 page.on('websocket',()=>sockets++);
 page.on('request',request=>{if(request.method()==='POST'&&request.url().endsWith('/visual/video/source'))uploads++;});
 await page.route('**/visual',route=>incompatible
  ? route.fulfill({status:500,headers:{'X-Visual-Error':'schema'},body:'PRIVATE SQL /secret'}) : route.continue());
 await page.goto('/'); await expect(page.locator('#status')).toHaveText('Connected');
 await page.keyboard.press('t'); await page.keyboard.insertText('Keep Visual recovery draft 界');
 await openVisual(page);
 await expect(screen(page)).toContainText('Database incompatible');
 await expect(screen(page)).toContainText('repair required');
 await clickControl(page,'Choose image or video');
 await expect(screen(page)).toContainText('Database incompatible'); expect(uploads).toBe(0);
 await expect(screen(page)).not.toContainText('PRIVATE');
 incompatible=false;
 await clickControl(page,'Back'); await clickControl(page,'Visual');
 await expect(screen(page)).toContainText('JPEG/PNG');
 await chooseImage(page,{name:'synthetic.mp4',mimeType:'video/mp4',buffer:readFileSync(new URL('./fixtures/visual-video.mp4',import.meta.url))});
 const before=await(await page.request.get('/visual')).json();
 await page.route('**/visual/video',route=>rejectApply
  ? route.fulfill({status:500,headers:{'X-Visual-Error':'settings'},body:'PRIVATE SQL /secret'}) : route.continue());
 await clickControl(page,'Apply');
 await expect(screen(page)).toContainText('settings in the database');
 await expect(screen(page)).toContainText('Previewed:');
 expect(await(await page.request.get('/visual')).json()).toEqual(before);
 rejectApply=false;
 await clickControl(page,'Apply');
 await expect(screen(page)).not.toContainText('Background darkness:');
 await expect(screen(page)).toContainText('Keep Visual recovery draft 界');
 expect(sockets).toBe(1);
 await page.screenshot({path:testInfo.outputPath('recovered-draft.png')});
});
