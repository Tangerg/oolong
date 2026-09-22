import {createRequire} from 'node:module';
import {pathToFileURL} from 'node:url';
import {readFile, writeFile} from 'node:fs/promises';

const [entry, configPath, output] = process.argv.slice(2);
const config = JSON.parse(await readFile(configPath, 'utf8'));
const requireCLI = createRequire(entry);
const puppeteerPath = requireCLI.resolve('puppeteer');
const {default: puppeteer} = await import(pathToFileURL(puppeteerPath));
const {launch, CDP_WEBSOCKET_ENDPOINT_REGEX} = await import(
  pathToFileURL(createRequire(puppeteerPath).resolve('@puppeteer/browsers'))
);
const {renderMermaid} = await import(pathToFileURL(requireCLI.resolve('@mermaid-js/mermaid-cli')));
process.stdin.setEncoding('utf8');
let source = '';
for await (const chunk of process.stdin) source += chunk;

// The Go owner creates the process group/job. Browser launch must never create
// a detached group: cleanup cannot depend on this JavaScript remaining responsive.
// Keep both executable selection and arguments on the official CLI's browser mode.
const browserOptions = {headless: 'shell', userDataDir: config.profile};
const processHandle = launch({
  executablePath: config.browser || await puppeteer.executablePath(browserOptions),
  args: [...await puppeteer.defaultArgs(browserOptions), '--remote-debugging-port=0'],
  detached: false,
  handleSIGINT: false,
  handleSIGTERM: false,
  handleSIGHUP: false,
});
let browser;
try {
  const endpoint = await processHandle.waitForLineOutput(CDP_WEBSOCKET_ENDPOINT_REGEX, config.timeout);
  browser = await puppeteer.connect({browserWSEndpoint: endpoint});
  browser.on('targetcreated', async target => {
    try {
      const page = await target.page();
      if (page) page.on('requestfailed', request => console.error(JSON.stringify({
        page: page.url(), url: request.url(), failure: request.failure(),
      })));
    } catch (error) { console.error('mermaid: page diagnostics:', error); }
  });
  const {data} = await renderMermaid(browser, source, 'png', {
    viewport: {width: config.width, height: config.height},
    backgroundColor: 'transparent',
    mermaidConfig: config.mermaid,
  });
  await writeFile(output, data, {mode: 0o600});
} finally {
  try { if (browser) await browser.close(); }
  finally {
    // Failed spawn has no exit event; Puppeteer's close waits for that event.
    if (processHandle.nodeProcess.pid) await processHandle.close();
  }
}
