const { chromium } = require('playwright');
(async () => {
    const browser = await chromium.launch({ headless: true });
    const page = await browser.newPage();
    page.on('console', msg => console.log('PAGE:', msg.text()));
    await page.goto('http://localhost:8080');
    await page.evaluate(async () => {
        try {
            const search = await import("https://esm.sh/@codemirror/search@6.5.6");
            window.editor.dispatch({
                effects: window.editor.state.update({
                    effects: []
                })
            });
            console.log("imported search!", typeof search.search);
        } catch(e) {
            console.error("Crash!", e);
        }
    });
    await browser.close();
})();
