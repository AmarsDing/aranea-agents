import { test, expect } from '@playwright/test';

test.describe('Chat Flow', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
  });

  test('should display the chat interface', async ({ page }) => {
    await expect(page.locator('[data-testid="chat-input"], .chat-input, textarea').first()).toBeVisible({ timeout: 10000 });
  });

  test('should create a new session and send a message', async ({ page }) => {
    // Look for new session button
    const newSessionBtn = page.locator('[data-testid="new-session"], button:has-text("New"), .q-btn:has-text("新")').first();
    if (await newSessionBtn.isVisible()) {
      await newSessionBtn.click();
    }

    // Find and fill chat input
    const chatInput = page.locator('[data-testid="chat-input"], textarea, [contenteditable="true"]').first();
    await expect(chatInput).toBeVisible({ timeout: 10000 });
    await chatInput.fill('Hello, agent!');

    // Find and click send button
    const sendBtn = page.locator('[data-testid="send-button"], button:has-text("Send"), button[aria-label*="send"], button[aria-label*="发送"]').first();
    if (await sendBtn.isVisible()) {
      await sendBtn.click();
    } else {
      await chatInput.press('Enter');
    }

    // Wait for response (assistant message or any new content)
    await page.waitForTimeout(5000);
  });
});
