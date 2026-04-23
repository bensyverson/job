#!/usr/bin/env node
/*
  Renders a local HTML file (or a URL) in headless Chromium and writes a PNG.
  Intended for the web-dashboard prototype; viewport defaults match the
  DESIGN.md "wide-viewport desktop primary target."

  Usage:
    node index.mjs <input> <output> [--width=1440] [--height=900] [--full]
*/

import { chromium } from "playwright";
import { resolve, isAbsolute } from "node:path";
import { pathToFileURL } from "node:url";
import { existsSync } from "node:fs";

function parseArgs(argv) {
  const positional = [];
  const opts = { width: 1440, height: 900, full: false };
  for (const arg of argv) {
    if (arg.startsWith("--width=")) opts.width = Number(arg.slice(8));
    else if (arg.startsWith("--height=")) opts.height = Number(arg.slice(9));
    else if (arg === "--full") opts.full = true;
    else positional.push(arg);
  }
  return { positional, opts };
}

async function main() {
  const { positional, opts } = parseArgs(process.argv.slice(2));
  if (positional.length < 2) {
    console.error("usage: screenshot <input> <output> [--width=N] [--height=N] [--full]");
    process.exit(2);
  }
  const [input, output] = positional;

  let url;
  if (input.startsWith("http://") || input.startsWith("https://")) {
    url = input;
  } else {
    const abs = isAbsolute(input) ? input : resolve(process.cwd(), input);
    if (!existsSync(abs)) {
      console.error(`input not found: ${abs}`);
      process.exit(1);
    }
    url = pathToFileURL(abs).href;
  }

  const browser = await chromium.launch();
  const context = await browser.newContext({
    viewport: { width: opts.width, height: opts.height },
    deviceScaleFactor: 2,
    colorScheme: "dark",
  });
  const page = await context.newPage();
  await page.goto(url, { waitUntil: "load" });
  // Give fonts + any DOMContentLoaded paint passes a tick to settle.
  await page.waitForTimeout(150);
  await page.screenshot({ path: output, fullPage: opts.full });
  await browser.close();

  console.log(output);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
