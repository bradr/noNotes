const { chromium } = require('playwright');
(async () => {
    const browser = await chromium.launch({ headless: true });
    const page = await browser.newPage();
    page.on('console', msg => {
        if (msg.type() === 'error') console.log('ERROR:', msg.text());
        else console.log('LOG:', msg.text());
    });
    page.on('pageerror', err => console.log('PAGE ERROR:', err.message));
    await page.goto('http://localhost:8080');
    await page.waitForTimeout(1000);
    await browser.close();
})();
