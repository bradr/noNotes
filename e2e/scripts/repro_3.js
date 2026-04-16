const { chromium } = require('playwright');
(async () => {
    const browser = await chromium.launch({ headless: true });
    const page = await browser.newPage();
    await page.goto('http://localhost:8080');
    await page.waitForTimeout(1000);
    await page.keyboard.press('Meta+f');
    await page.waitForTimeout(500);
    const searchPanel = await page.$('.cm-search');
    console.log("Search panel exists?", !!searchPanel);
    const html = await page.evaluate(() => document.querySelector('.cm-search')?.outerHTML);
    console.log("Search panel HTML:", html);
    await browser.close();
})();
