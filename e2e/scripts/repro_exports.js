const { chromium } = require('playwright');
(async () => {
    const browser = await chromium.launch({ headless: true });
    const page = await browser.newPage();
    await page.goto('http://localhost:8080');
    const exports = await page.evaluate(async () => {
        const mod = await import("https://esm.sh/codemirror@6.0.1");
        return Object.keys(mod);
    });
    console.log(exports);
    await browser.close();
})();
