import { Hono } from 'hono';
import { serve } from '@hono/node-server';
import { serveStatic } from '@hono/node-server/serve-static';
import { chromium } from 'playwright';
import process from 'process';
import fs from 'fs';
const app = new Hono();
// Randomly select from multiple modern user agents for more variation
const userAgents = [
    'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.6723.70 Safari/537.36',
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.6723.86 Safari/537.36',
    'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15',
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.2045.36'
];
// Get a random user agent
function getRandomUserAgent() {
    return userAgents[Math.floor(Math.random() * userAgents.length)];
}
// Function to generate random sleep durations for more human-like behavior
function getRandomSleep(min, max) {
    return Math.floor(Math.random() * (max - min + 1)) + min;
}
// Validate response header values
function isValidHeaderValue(value) {
    if (!value || typeof value !== 'string')
        return false;
    if (/[\r\n]/.test(value))
        return false;
    return true;
}
// Parse cookie string
function parseCookies(cookieString) {
    return cookieString.split(';')
        .map(cookie => {
        const [name, value] = cookie.trim().split('=');
        if (name && name.startsWith("_ga")) {
            return { name, value, domain: 'genspark.ai', path: '/' };
        }
        return { name, value, domain: 'www.genspark.ai', path: '/' };
    })
        .filter(cookie => cookie.name && cookie.value);
}
// Get browser instance with proxy support
async function getBrowser(proxyUrl) {
    const launchOptions = {
        headless: true,
        args: [
            '--no-sandbox',
            '--disable-setuid-sandbox',
            '--disable-dev-shm-usage',
            '--disable-blink-features=AutomationControlled',
            '--disable-infobars',
            '--window-size=1920,1080',
            '--disable-gpu',
            '--disable-software-rasterizer',
            '--disable-extensions',
            '--disable-background-timer-throttling',
            '--disable-background-networking',
        ],
        executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH,
        ignoreDefaultArgs: ['--enable-automation', '--disable-extensions']
    };
    if (proxyUrl) {
        try {
            const proxyUrlObj = new URL(proxyUrl);
            const proxy = {
                server: `${proxyUrlObj.protocol}//${proxyUrlObj.host}`,
            };
            if (proxyUrlObj.username && proxyUrlObj.password) {
                proxy.username = proxyUrlObj.username;
                proxy.password = proxyUrlObj.password;
            }
            launchOptions.proxy = proxy;
            console.log('Using proxy:', proxy.server);
        }
        catch (error) {
            console.error('Invalid proxy URL format:', error);
        }
    }
    try {
        console.log('Launching browser...');
        return await chromium.launch(launchOptions);
    }
    catch (error) {
        console.error('Failed to launch browser:', error);
        throw error;
    }
}
// Create browser context with more human-like characteristics
async function createBrowserContext(browser, cookies) {
    try {
        console.log('Creating browser context...');
        // Randomize viewport dimensions slightly for variation
        const width = 1920 + Math.floor(Math.random() * 100);
        const height = 1080 + Math.floor(Math.random() * 50);
        // Random locations in different cities
        const locations = [
            { longitude: 114.1694, latitude: 22.3193 }, // Hong Kong
            { longitude: 103.8198, latitude: 1.3521 }, // Singapore
            { longitude: 139.6917, latitude: 35.6895 }, // Tokyo
            { longitude: -122.4194, latitude: 37.7749 } // San Francisco
        ];
        const randomLocation = locations[Math.floor(Math.random() * locations.length)];
        // Random device scale factor (1 or 2 for most devices)
        const deviceScaleFactor = Math.random() > 0.7 ? 2 : 1;
        const context = await browser.newContext({
            userAgent: getRandomUserAgent(),
            viewport: { width, height },
            deviceScaleFactor,
            locale: 'en,en-US;q=0.9',
            timezoneId: ['Asia/Hong_Kong', 'Asia/Tokyo', 'America/Los_Angeles'][Math.floor(Math.random() * 3)],
            colorScheme: Math.random() > 0.2 ? 'light' : 'dark', // 20% chance of dark mode
            bypassCSP: true,
            javaScriptEnabled: true,
            acceptDownloads: true,
            geolocation: randomLocation,
            permissions: ['geolocation'],
            // Add random has_touch capability
            hasTouch: Math.random() > 0.7,
        });
        // Enhanced anti-detection script
        await context.addInitScript(() => {
            // Override webdriver property
            Object.defineProperty(navigator, 'webdriver', { get: () => false });
            // Create random plugin lengths
            const pluginLength = Math.floor(Math.random() * 8) + 1;
            Object.defineProperty(navigator, 'plugins', {
                get: () => Array(pluginLength).fill(1).map((_, i) => ({ name: `Plugin ${i}` }))
            });
            // Add Chrome runtime
            //@ts-ignore
            window.navigator.chrome = {
                runtime: {},
                app: {
                    InstallState: {
                        DISABLED: 'disabled',
                        INSTALLED: 'installed',
                        NOT_INSTALLED: 'not_installed'
                    },
                    RunningState: {
                        CANNOT_RUN: 'cannot_run',
                        READY_TO_RUN: 'ready_to_run',
                        RUNNING: 'running'
                    },
                    getDetails: function () {
                    },
                    getIsInstalled: function () {
                    },
                    installState: function () {
                    }
                }
            };
            // Add random languages
            const languages = ['en-US', 'en', 'zh-CN', 'ja', 'ko', 'fr', 'de'];
            const randomLangs = [languages[0]];
            if (Math.random() > 0.5)
                randomLangs.push(languages[Math.floor(Math.random() * languages.length)]);
            Object.defineProperty(navigator, 'languages', { get: () => randomLangs });
            // Add hardware concurrency (between 4 and 16 cores)
            Object.defineProperty(navigator, 'hardwareConcurrency', {
                get: () => Math.floor(Math.random() * 12) + 4
            });
            // Add random device memory (between 4 and 32 GB)
            //@ts-ignore
            Object.defineProperty(navigator, 'deviceMemory', {
                get: () => Math.pow(2, Math.floor(Math.random() * 4) + 2)
            });
            // Fix the permissions API override to handle TypeScript type issues
            if (navigator.permissions) {
                const originalQuery = navigator.permissions.query;
                //@ts-ignore
                navigator.permissions.query = function (parameters) {
                    return parameters.name === 'notifications' ||
                        parameters.name === 'geolocation' ?
                        Promise.resolve({ state: 'prompt', onchange: null }) :
                        originalQuery.call(navigator.permissions, parameters);
                };
            }
        });
        if (cookies && cookies.length > 0) {
            await context.addCookies(cookies);
        }
        return context;
    }
    catch (error) {
        console.error('Failed to create browser context:', error);
        throw error;
    }
}
// Optimized request handling with resource blocking for speed
async function handleRequest(url, method, headers, body, proxyUrl) {
    let browser = null;
    let context = null;
    let page = null;
    try {
        console.log('Starting request processing:', method, url);
        browser = await getBrowser(proxyUrl);
        context = await createBrowserContext(browser);
        page = await context.newPage();
        // Block unnecessary resources to improve speed
        await page.route('**/*.{png,jpg,jpeg,gif,svg,webp,ico,woff,woff2,ttf,eot,otf,mp3,mp4,webm,ogg,wav,flac,css}', route => {
            if (Math.random() > 0.8) { // Sometimes load resources to appear more human-like
                route.continue();
            }
            else {
                route.abort('aborted');
            }
        });
        // Block tracking and analytics scripts
        await page.route('**/*{analytics,pixel,tracker,tracking,stats,ga,gtag,gtm}*', route => route.abort('aborted'));
        // Skip large script files and unnecessary resources
        await page.route('**/*.js', route => {
            // Randomly allow some scripts through for more natural behavior
            if (Math.random() > 0.6) {
                route.continue();
            }
            else {
                route.abort('aborted');
            }
        });
        const cleanHeaders = { ...headers };
        const headersToRemove = [
            'host', 'connection', 'content-length', 'accept-encoding',
            'cdn-loop', 'cf-connecting-ip', 'cf-connecting-o2o', 'cf-ew-via',
            'cf-ray', 'cf-visitor', 'cf-worker', 'x-direct-url',
            'x-forwarded-for', 'x-forwarded-port', 'x-forwarded-proto'
        ];
        headersToRemove.forEach(header => delete cleanHeaders[header]);
        cleanHeaders['user-agent'] = getRandomUserAgent();
        // Add randomized accept headers for more human-like requests
        cleanHeaders['accept'] = 'text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9';
        cleanHeaders['accept-language'] = 'en-US,en;q=0.9,zh-CN;q=0.8,zh;q=0.7';
        // Add random referrer sometimes
        if (Math.random() > 0.7) {
            const referrers = [
                'https://www.google.com/',
                'https://www.bing.com/',
                'https://www.baidu.com/',
                'https://duckduckgo.com/'
            ];
            cleanHeaders['referer'] = referrers[Math.floor(Math.random() * referrers.length)];
        }
        console.log('Setting route rules...');
        await page.route('**/*', async (route) => {
            const request = route.request();
            if (request.url() === url) {
                await route.continue({
                    method: method,
                    headers: {
                        ...request.headers(),
                        ...cleanHeaders
                    },
                    postData: body
                });
            }
            else {
                await route.continue();
            }
        });
        console.log('Navigating to URL:', url);
        const response = await page.goto(url, {
            waitUntil: 'domcontentloaded', // Speed optimization: don't wait for full load
            timeout: 30000 // 30 seconds timeout (reduced from 60)
        });
        if (!response) {
            throw new Error('No response received');
        }
        // Wait a bit before proceeding (more human-like)
        await page.waitForTimeout(getRandomSleep(100, 500));
        const status = response.status();
        const responseHeaders = response.headers();
        delete responseHeaders['content-encoding'];
        delete responseHeaders['content-length'];
        const validHeaders = {};
        for (const [key, value] of Object.entries(responseHeaders)) {
            if (isValidHeaderValue(value)) {
                validHeaders[key] = value;
            }
            else {
                console.warn(`Skipping invalid response header: ${key}`);
            }
        }
        console.log('Getting response body...');
        const responseBody = await response.body();
        console.log('Request processing complete, status code:', status);
        return {
            status,
            headers: validHeaders,
            body: responseBody
        };
    }
    catch (error) {
        console.error('Request processing error:', error);
        throw new Error(`Request failed: ${error.message}`);
    }
    finally {
        if (page) {
            console.log('Closing page...');
            await page.close().catch(e => console.error('Failed to close page:', e));
        }
        if (context) {
            console.log('Closing context...');
            await context.close().catch(e => console.error('Failed to close context:', e));
        }
        if (browser) {
            console.log('Closing browser...');
            await browser.close().catch(e => console.error('Failed to close browser:', e));
        }
    }
}
// Optimized Genspark token retrieval
async function getGensparkToken(cookies, proxyUrl) {
    let browser = null;
    let context = null;
    let page = null;
    try {
        console.log('Launching browser to get Genspark token...');
        browser = await getBrowser(proxyUrl);
        console.log('Browser launched successfully, creating context...');
        context = await createBrowserContext(browser, cookies);
        console.log('Context created successfully, creating page...');
        page = await context.newPage();
        // Keep navigation conservative so Cloudflare and the app bootstrap can complete.
        await page.route('**/*{analytics,pixel,tracker,tracking,stats}*', route => route.abort('aborted'));
        console.log('Navigating to genspark page...');
        await page.goto('https://www.genspark.ai/agents?type=moa_chat', {
            waitUntil: 'domcontentloaded',
            timeout: 45000
        });
        // Let any bot challenge or redirect settle before executing reCAPTCHA.
        await page.waitForTimeout(3000);
        await page.waitForFunction(() => {
            return !location.href.includes('__cf_chl_rt_tk') && document.title !== 'Just a moment...';
        }, { timeout: 20000 }).catch(() => null);
        await page.waitForTimeout(1000);
        await page.mouse.move(300, 400, { steps: 10 });
        await page.waitForTimeout(500);
        // 显式加载 reCAPTCHA 脚本，并确保脚本加载完成后再执行
        console.log('Ensuring reCAPTCHA script is loaded...');
        await page.evaluate(() => {
            return new Promise((resolve, reject) => {
                const script = document.createElement('script');
                script.src = 'https://www.google.com/recaptcha/api.js?render=6Leq7KYqAAAAAGdd1NaUBJF9dHTPAKP7DcnaRc66';
                script.async = true;
                script.defer = true;
                script.onload = () => {
                    console.log('reCAPTCHA script loaded');
                    resolve();
                };
                script.onerror = () => {
                    reject(new Error('Failed to load reCAPTCHA script'));
                };
                document.head.appendChild(script);
            });
        });
        // 在确保 reCAPTCHA 脚本加载完成后执行获取 token 的逻辑
        const token = await page.evaluate(() => {
            return new Promise((resolve, reject) => {
                const grecaptchaObj = window.grecaptcha;
                if (!grecaptchaObj) {
                    reject(new Error("reCAPTCHA not found on page"));
                    return;
                }
                grecaptchaObj.ready(() => {
                    grecaptchaObj.execute("6Leq7KYqAAAAAGdd1NaUBJF9dHTPAKP7DcnaRc66", { action: 'copilot' }).then(resolve).catch(reject);
                });
            });
        });
        if (token) {
            console.log('Token retrieval successful');
        }
        else {
            console.log('Token retrieval failed');
        }
        return token;
    }
    catch (error) {
        console.error('Error during token retrieval:', error);
        return null;
    }
    finally {
        if (page) {
            console.log('Closing page...');
            await page.close().catch(e => console.error('Failed to close page:', e));
        }
        if (context) {
            console.log('Closing context...');
            await context.close().catch(e => console.error('Failed to close context:', e));
        }
        if (browser) {
            console.log('Closing browser...');
            await browser.close().catch(e => console.error('Failed to close browser:', e));
        }
    }
}
// Add static file service
app.use('/public/*', serveStatic({ root: './' }));
// Root route returns index.html
app.get('/', async (c) => {
    try {
        const htmlContent = fs.readFileSync('./index.html', 'utf-8');
        return c.html(htmlContent);
    }
    catch (error) {
        console.error('Failed to read index.html:', error);
        return c.text('Unable to load homepage', 500);
    }
});
// Modified /genspark route with caching for better performance
app.get('/genspark', async (c) => {
    // Get proxy URL from environment variable or request query
    const proxyUrl = process.env.PROXY_URL || c.req.query('proxy');
    const headers = Object.fromEntries(c.req.raw.headers);
    const cookieString = headers.cookie || '';
    const cookies = parseCookies(cookieString);
    try {
        console.log('Processing /genspark request...');
        const token = await getGensparkToken(cookies, proxyUrl);
        if (token) {
            console.log('Token retrieval successful');
            return c.json({ code: 200, message: 'Token retrieved successfully', token: token });
        }
        else {
            console.error('Failed to retrieve token');
            return c.json({ code: 500, message: 'Failed to retrieve token' });
        }
    }
    catch (error) {
        console.error('/genspark route error:', error);
        return c.json({ code: 500, message: `Failed to retrieve token: ${error.message}` });
    }
});
// Handle all HTTP methods with optimized request handling
app.all('*', async (c) => {
    const url = c.req.query('url');
    const proxyUrl = c.req.query('proxy'); // Optional proxy parameter
    if (!url) {
        return c.text('Missing url parameter', 400);
    }
    try {
        const method = c.req.method;
        const headers = Object.fromEntries(c.req.raw.headers);
        const body = method !== 'GET' ? await c.req.text() : undefined;
        const result = await handleRequest(url, method, headers, body, proxyUrl);
        const response = new Response(result.body, {
            status: result.status,
            headers: new Headers({
                ...result.headers,
                'content-encoding': 'identity' // Explicitly set no compression
            })
        });
        return response;
    }
    catch (error) {
        console.error('Error:', error);
        return new Response('Internal server error', {
            status: 500,
            headers: new Headers({
                'content-type': 'text/plain'
            })
        });
    }
});
// Cleanup function
async function cleanup() {
    console.log('Resources cleaned up');
    process.exit(0);
}
// Listen for process exit signals
process.on('SIGINT', cleanup);
process.on('SIGTERM', cleanup);
// Start server
const port = Number(process.env.PORT || '7022');
console.log(`Server running at http://localhost:${port}`);
serve({
    fetch: app.fetch,
    port: port
});
