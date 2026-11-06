const assert = require('node:assert/strict');
const fs = require('node:fs');
const test = require('node:test');
const { Terminal } = require('@xterm/headless');

const scenarios = JSON.parse(fs.readFileSync(0, 'utf8'));

function write(terminal, output) {
  return new Promise(resolve => terminal.write(output, resolve));
}

function snapshot(terminal) {
  const buffer = terminal.buffer.active;
  return {
    cursor: [buffer.cursorX, buffer.cursorY],
    base: buffer.baseY,
    rows: Array.from({ length: buffer.length }, (_, row) => {
      const line = buffer.getLine(row);
      return [line.translateToString(true), line.isWrapped];
    }),
  };
}

function historyBefore(buffer, end) {
  const lines = [];
  for (let row = 0; row < end; row++) {
    const line = buffer.getLine(row);
    const text = line.translateToString(true);
    if (row && line.isWrapped) {
      lines[lines.length - 1] += text;
    } else {
      lines.push(text);
    }
  }
  return lines.map(line => line.trimEnd());
}

function assertDisplay(terminal, expected, label) {
  const buffer = terminal.buffer.active;
  const live = expected.live || [];
  const cursor = buffer.baseY + buffer.cursorY;
  const top = cursor - expected.cursor.Y;
  assert.equal(buffer.cursorX, expected.cursor.X, `${label}: cursor column`);
  assert(top >= buffer.baseY, `${label}: live block must remain in the viewport`);
  assert.deepEqual(historyBefore(buffer, top), expected.history, `${label}: preserve history exactly once`);
  for (let row = 0; row < live.length; row++) {
    const line = buffer.getLine(top + row);
    assert.equal(line.translateToString(true).trimEnd(), live[row], `${label}: live row ${row}`);
    assert.equal(line.isWrapped, row > 0, `${label}: live row ${row} stays in one logical line`);
  }
  for (let row = top + live.length; row < buffer.length; row++) {
    assert.equal(buffer.getLine(row).translateToString(true).trimEnd(), '', `${label}: stale row ${row}`);
  }
  for (let row = top - expected.hard; row < top; row++) {
    assert.equal(buffer.getLine(row).isWrapped, false, `${label}: finished row has a hard boundary`);
  }
}

for (const scenario of scenarios) {
  test(scenario.name, async () => {
    const terminal = new Terminal({
      cols: scenario.size.columns,
      rows: scenario.size.rows,
      scrollback: 1000,
      allowProposedApi: true,
    });
    try {
      if (scenario.history.length) {
        await write(terminal, scenario.history.join('\r\n') + '\r\n');
      }
      for (const step of scenario.steps) {
        const label = `${scenario.name}, ${step.name}`;
        if (step.resize) {
          terminal.resize(step.resize.columns, step.resize.rows);
        }
        const before = snapshot(terminal);
        if (step.unchanged) {
          assert.equal(step.output || '', '', `${label}: no wire output`);
        }
        if (step.output) {
          await write(terminal, step.output);
        }
        if (step.unchanged) {
          assert.deepEqual(snapshot(terminal), before, `${label}: terminal state is unchanged`);
        }
        if (step.want) {
          assertDisplay(terminal, step.want, label);
        }
      }
    } finally {
      terminal.dispose();
    }
  });
}
