import { Before, After, BeforeAll, AfterAll, Status, setDefaultTimeout } from '@cucumber/cucumber';
import { Browser, chromium } from 'playwright';
import { ICustomWorld } from './world.ts';
import * as dotenv from 'dotenv';

// Load environment variables
dotenv.config({ path: '.env.local' });

// Increase default step timeout for browser-based tests (default is 5000ms)
setDefaultTimeout(30000);

let browser: Browser;

BeforeAll(async function () {
  const isHeaded = process.env.HEADED === 'true';
  const launchOptions: Parameters<typeof chromium.launch>[0] = {
    headless: !isHeaded,
  };

  // Slow down actions in headed mode so we can see what's happening
  if (isHeaded) {
    launchOptions.slowMo = parseInt(process.env.SLOW_MO || '250', 10);
  }

  // Use executablePath for system browser (required on NixOS)
  // Falls back to channel for branded browsers like 'chrome' or 'msedge'
  if (process.env.BROWSER_PATH) {
    launchOptions.executablePath = process.env.BROWSER_PATH;
  } else if (process.env.BROWSER_CHANNEL) {
    launchOptions.channel = process.env.BROWSER_CHANNEL;
  }

  browser = await chromium.launch(launchOptions);
});

AfterAll(async function () {
  await browser?.close();
});

Before(async function (this: ICustomWorld) {
  this.browser = browser;
  this.context = await browser.newContext({
    baseURL: process.env.BASE_URL || 'http://console.127.0.0.1.nip.io:8080',
    viewport: { width: 1280, height: 720 },
  });
  this.page = await this.context.newPage();
});

After(async function (this: ICustomWorld, { result }) {
  // Capture screenshot on failure
  if (result?.status === Status.FAILED && this.page) {
    const screenshot = await this.page.screenshot();
    this.attach(screenshot, 'image/png');
  }

  await this.page?.close();
  await this.context?.close();
});
