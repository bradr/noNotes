const { test, expect } = require('@playwright/test');

test('Page loads without console errors or exceptions', async ({ page }) => {
    const errors = [];
    const consoleErrors = [];

    // Listen for uncaught exceptions
    page.on('pageerror', (exception) => {
        errors.push(`Uncaught Exception: ${exception.message}`);
    });

    // Listen for console.error calls
    page.on('console', (msg) => {
        if (msg.type() === 'error') {
            consoleErrors.push(`Console Error: ${msg.text()}`);
        }
    });

    // Listen for failed network requests (e.g., 404s for JS files)
    page.on('requestfailed', (request) => {
        errors.push(`Request Failed: ${request.url()} (${request.failure().errorText})`);
    });

    // Navigate to the app
    await page.goto('http://localhost:8080');

    // Wait for the editor to potentially throw async errors
    await page.waitForTimeout(2000);

    const allErrors = [...errors, ...consoleErrors];
    
    if (allErrors.length > 0) {
        throw new Error(`Found ${allErrors.length} errors in the console:\n\n${allErrors.join('\n')}`);
    }
});
