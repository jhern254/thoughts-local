import {test, expect} from '@playwright/test';
const screen = page => page.locator('.xterm-rows');
test('Goals workflow creates a target and returns to Events without reconnecting', async ({page}) => {
 let sockets=0;page.on('websocket',()=>sockets++);
 await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');
 await page.locator('.xterm-helper-textarea').focus();
 await page.keyboard.press('Tab');await page.keyboard.press('ArrowLeft');await page.keyboard.press('ArrowLeft');await page.keyboard.press('Enter');
 await expect(screen(page)).toContainText('Goals');
 await page.keyboard.press('n');await expect(screen(page)).toContainText('Create Goal');
 const name=`Goal browser ${Date.now()}`;await page.keyboard.type(name);
 await page.keyboard.press('Tab');await page.keyboard.type('1');await page.keyboard.press('Control+s');
 await expect(screen(page)).toContainText(name);await expect(screen(page)).toContainText('Manage Subjects');await expect(screen(page)).toContainText('Misc');
 const connections=sockets;
 await page.screenshot({path:`${process.env.THOUGHTS_BROWSER_ARTIFACTS}/goals-${test.info().project.name}-normal.png`});
 await page.keyboard.press('e');await expect(screen(page)).toContainText('Edit Goal');await page.keyboard.press('Escape');
 await page.keyboard.press('s');await expect(screen(page)).toContainText('Select Subjects');
 await page.keyboard.press('/');await page.keyboard.type('no-subject-matches-this-query');await expect(screen(page)).toContainText('No matches');await page.keyboard.press('Escape');await page.keyboard.press('Escape');
 await page.setViewportSize({width:700,height:450});await expect(screen(page)).toContainText(name);
 await page.screenshot({path:`${process.env.THOUGHTS_BROWSER_ARTIFACTS}/goals-${test.info().project.name}-narrow.png`});
 await page.keyboard.press('Escape');await expect(screen(page)).toContainText('Goals');await page.keyboard.press('Escape');await expect(screen(page)).toContainText('Events');
 expect(sockets).toBe(connections);
});

test('Goal Subject picker creates through the existing form and retains its draft', async ({page}, testInfo) => {
 await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');
 await page.locator('.xterm-helper-textarea').focus();
 await page.keyboard.press('Tab');await page.keyboard.press('ArrowLeft');await page.keyboard.press('ArrowLeft');await page.keyboard.press('Enter');
 await expect(screen(page)).toContainText('Goals');await page.keyboard.press('n');
 const goalName=`Draft Goal ${Date.now()}`;await page.keyboard.type(goalName);await page.keyboard.press('Tab');await page.keyboard.type('1');
 for(let i=0;i<5;i++)await page.keyboard.press('Tab');await page.keyboard.press('Enter');
 await expect(screen(page)).toContainText('Select Subjects');await expect(screen(page)).toContainText('Create subject…');
 await page.keyboard.press('Enter');await expect(screen(page)).toContainText('Subject name:');
 await page.keyboard.press('Escape');await expect(screen(page)).toContainText('Select Subjects');
 await page.keyboard.press('Enter');await expect(screen(page)).toContainText('Subject name:');
 const subjectName=`Created for Goal ${Date.now()}`;await page.keyboard.type(subjectName);await page.keyboard.press('Enter');
 await expect(screen(page)).toContainText('Select Subjects');await expect(screen(page)).toContainText(subjectName);await expect(screen(page)).toContainText('1 selected');
 await page.screenshot({path:`${process.env.THOUGHTS_BROWSER_ARTIFACTS}/subject-picker-${testInfo.project.name}.png`});
 await page.keyboard.press('Enter');await expect(screen(page)).toContainText('Create Goal');await expect(screen(page)).toContainText(goalName);
 await page.keyboard.press('Control+s');await expect(screen(page)).toContainText('Manage Subjects');await expect(screen(page)).toContainText(subjectName);
 await page.keyboard.press('Escape');await expect(screen(page)).toContainText('Goals');await page.keyboard.press('Escape');await expect(screen(page)).toContainText('Events');
 await page.keyboard.press('ArrowRight');await page.keyboard.press('ArrowRight');await page.keyboard.press('Enter');
 await expect(screen(page)).toContainText('Browse Subjects');await expect(screen(page)).toContainText('Create subject…');
 await page.screenshot({path:`${process.env.THOUGHTS_BROWSER_ARTIFACTS}/subjects-browse-${testInfo.project.name}.png`});
});

test('Goal panels navigate vertically and held Left stops before leaving the Goals list', async ({page}) => {
 let sockets=0;page.on('websocket',()=>sockets++);
 await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');
 await page.locator('.xterm-helper-textarea').focus();
 for(const key of ['Tab','ArrowLeft','ArrowLeft','Enter'])await page.keyboard.press(key);
 await expect(screen(page)).toContainText('Priority:');
 await page.keyboard.press('Tab');await page.keyboard.press('ArrowDown');await page.keyboard.press('ArrowRight');
 await expect(screen(page)).toContainText('/ Search...');
 await page.keyboard.press('ArrowLeft');await expect(screen(page)).toContainText('Priority:');
 await page.keyboard.press('ArrowLeft');await expect(screen(page)).toContainText('Priority:');
 await page.keyboard.press('ArrowUp');await expect(screen(page)).toContainText('h/l: status');
 await page.keyboard.press('ArrowDown');await page.keyboard.press('Tab');
 for(let i=0;i<3;i++) {
  await page.keyboard.press('n');await expect(screen(page)).toContainText('Create Goal');
  await page.keyboard.type(`Paging Goal ${Date.now()} ${i}`);await page.keyboard.press('Tab');await page.keyboard.type('1');
  await page.keyboard.press('Control+s');await expect(screen(page)).toContainText('Manage Subjects');
  await page.keyboard.press('Escape');await expect(screen(page)).toContainText('p: priority');
 }
 await page.setViewportSize({width:480,height:360});await expect(screen(page)).toContainText('p: priority · ↑: filter');
 await page.keyboard.press('End');
 for(let i=0;i<20;i++)await page.keyboard.down('ArrowLeft');
 await expect(screen(page)).toContainText('p: priority');
 await page.keyboard.up('ArrowLeft');await page.keyboard.press('ArrowLeft');
 await expect(screen(page)).toContainText('Events');
 expect(sockets).toBe(1);
});
