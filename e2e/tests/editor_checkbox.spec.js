const { test, expect } = require('@playwright/test');

test.describe('Editor Checkbox Toggle', () => {
    test('clicking a checkbox in the editor should toggle it', async ({ page }) => {
        await page.goto('/');
        await page.waitForTimeout(1000);

        const uniqueTask = `- [ ] Click Toggle ${Date.now()}`;
        await page.evaluate((text) => {
            window.editor.dispatch({
                changes: {from: window.editor.state.doc.length, insert: '\n' + text}
            });
        }, uniqueTask);

        // Find the line in the editor
        const line = page.locator(`.cm-line:has-text("Click Toggle")`).last();
        await expect(line).toBeVisible();

        // Click the beginning of the line (where the checkbox is)
        const box = await line.boundingBox();
        // Click slightly to the right to hit the [ ] part (approx 45px in)
        await page.mouse.click(box.x + 45, box.y + box.height / 2);

        // Check if it toggled
        await page.waitForTimeout(500);
        const content = await page.evaluate(() => window.editor.state.doc.toString());
        expect(content).toContain(uniqueTask.replace('[ ]', '[x]'));
    });
});
