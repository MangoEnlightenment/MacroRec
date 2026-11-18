// Renders the MacroRec mark as square app-icon PNGs (transparent corners).
const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');

const out = process.argv[2];
const sizes = [16, 24, 32, 48, 64, 128, 256];

function svg(S) {
  const small = S <= 48;
  const k = small ? 1.62 : 1.36;
  const t = (512 - 256 * k) / 2;
  const sw = small ? 16 : 9;
  const dash = small ? '1 33' : '1 20';
  const ring = small ? 38 : 30;
  const dot = small ? 24 : 18;
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${S}" height="${S}" viewBox="0 0 512 512">
<rect width="512" height="512" rx="118" fill="#0E1014"/>
<g transform="translate(${t} ${t}) scale(${k})"><g transform="translate(-7 -13)">
<path d="M64 190 C 64 120 100 70 168 62" fill="none" stroke="#6D8DFF" stroke-width="${sw}" stroke-linecap="round" stroke-dasharray="${dash}"/>
<circle cx="64" cy="190" r="${ring}" fill="#FF5468" fill-opacity="0.22"/>
<circle cx="64" cy="190" r="${dot}" fill="#FF5468"/>
<polygon transform="translate(168 62) scale(5)" points="0,0 0,17 4.6,12.8 7.6,19.6 10.4,18.4 7.5,11.7 13.5,11.5" fill="#6D8DFF" stroke="#0E1014" stroke-width="1.2" stroke-linejoin="round"/>
</g></g></svg>`;
}

(async () => {
  fs.mkdirSync(out, { recursive: true });
  const browser = await chromium.launch();
  for (const S of sizes) {
    const page = await browser.newPage({ viewport: { width: S, height: S }, deviceScaleFactor: 1 });
    await page.setContent(`<html><body style="margin:0;background:transparent">${svg(S)}</body></html>`);
    await page.screenshot({ path: path.join(out, `icon-${S}.png`), omitBackground: true });
    await page.close();
  }
  await browser.close();
  console.log('rendered', sizes.join(','));
})();
