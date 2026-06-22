import { chromium } from '@playwright/test';

(async () => {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext();
  const page = await context.newPage();
  page.on('console', msg => console.log('CONSOLE:', msg.type(), msg.text()));
  page.on('pageerror', err => console.log('PAGEERROR:', err.message));

  await page.goto('http://localhost:9003/#/agents');
  await page.waitForTimeout(4000);

  // Check avatar images in agent cards
  const avatars = await page.locator('.agent-card .q-avatar img, .agent-card .resolved-avatar-img').all();
  console.log('Agent card avatar img count:', avatars.length);
  for (let i = 0; i < Math.min(avatars.length, 5); i++) {
    const src = await avatars[i].getAttribute('src');
    console.log(`Avatar ${i}:`, src ? src.slice(0, 80) : 'no src');
  }

  // Check any data: src images
  const dataImgs = await page.locator('img[src^="data:"]').all();
  console.log('Total data: images:', dataImgs.length);

  // Take screenshot
  await page.screenshot({ path: 'F:/aranea-agents/tmp/playwright-agents.png' });

  await browser.close();
})();
