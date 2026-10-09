import { glob, readFile } from "node:fs/promises";
import { lint } from "markdownlint/promise";

const config = JSON.parse(await readFile(new URL("../.markdownlint.json", import.meta.url), "utf8"));
const files = [];
for await (const file of glob("**/*.md", { exclude: ["**/node_modules/**"] })) {
  files.push(file);
}
files.sort();
const results = await lint({ files, config });
for (const [file, errors] of Object.entries(results)) {
  for (const error of errors) {
    console.error(`${file}:${error.lineNumber} ${error.ruleNames[0]} ${error.ruleDescription}${error.errorDetail ? ` [${error.errorDetail}]` : ""}`);
    process.exitCode = 1;
  }
}
if (!process.exitCode) {
  console.log(`Checked ${files.length} Markdown files`);
}
